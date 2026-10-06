/*
Copyright 2026 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package workload

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	// methodField matches the HTTP verb in KubeArchive's structured (slog) access
	// logs, where the verb is a discrete field: `... method=GET ...`.
	methodField = regexp.MustCompile(`\bmethod=(GET|POST|PUT|PATCH|DELETE)\b`)
	// pathField matches the request-target field in structured logs, either quoted
	// (`path="/apis/..."`) or bare (`path=/apis/...`). The path field is separate
	// from the method field, so the two are extracted independently.
	pathField = regexp.MustCompile(`\bpath="([^"]*)"|\bpath=(\S+)`)
	// requestLine matches the simpler inline `VERB /path` form some access logs use.
	// It is the fallback when the structured method/path fields are absent.
	requestLine = regexp.MustCompile(`\b(GET|POST|PUT|PATCH|DELETE)\s+(/\S*)`)
)

// Derive scans KubeArchive access-log lines from r and produces an anonymized
// QueryMixSpec. Only read (GET) requests against Kubernetes-style resource paths
// contribute; write verbs and non-API paths (health, metrics) are ignored.
//
// The returned spec records kinds, label-selector keys, page sizes, and namespace
// fan-out as counts and weights only. Raw namespaces, label values, tokens,
// hostnames, and UIDs are never copied into the spec.
func Derive(r io.Reader) (*QueryMixSpec, error) {
	ops := newCounter()       // keyed by "type|kind"
	pageSizes := newCounter() // keyed by limit as string
	labelKeys := newCounter() // keyed by label key
	namespaces := newCounter()

	opMeta := map[string]OperationWeight{}
	var listRequests, selectorRequests int

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		req, ok := parseRequest(scanner.Text())
		if !ok {
			continue
		}
		key := string(req.op) + "|" + req.kind
		ops.add(key)
		if _, seen := opMeta[key]; !seen {
			opMeta[key] = OperationWeight{Type: req.op, Kind: req.kind}
		}
		if req.namespace != "" {
			namespaces.add(req.namespace)
		}
		if req.limit > 0 {
			pageSizes.add(strconv.Itoa(req.limit))
		}
		if req.isCollection {
			listRequests++
			if len(req.labelKeys) > 0 {
				selectorRequests++
			}
		}
		for _, k := range req.labelKeys {
			labelKeys.add(k)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading access log: %w", err)
	}

	spec := &QueryMixSpec{
		SchemaVersion: SchemaVersion,
		SampleSize:    ops.total,
	}
	for _, key := range ops.keysByFrequency() {
		m := opMeta[key]
		m.Count = ops.counts[key]
		m.Weight = ops.weight(m.Count)
		spec.Operations = append(spec.Operations, m)
	}
	for _, limit := range pageSizes.keysByFrequency() {
		n, _ := strconv.Atoi(limit)
		spec.PageSizes = append(spec.PageSizes, PageSizeWeight{
			Limit:  n,
			Count:  pageSizes.counts[limit],
			Weight: pageSizes.weight(pageSizes.counts[limit]),
		})
	}
	if listRequests > 0 {
		spec.LabelSelector.Fraction = float64(selectorRequests) / float64(listRequests)
	}
	for _, k := range labelKeys.keysByFrequency() {
		spec.LabelSelector.Keys = append(spec.LabelSelector.Keys, LabelKeyWeight{
			Key:    k,
			Count:  labelKeys.counts[k],
			Weight: labelKeys.weight(labelKeys.counts[k]),
		})
	}
	spec.NamespaceFanout = deriveFanout(namespaces)
	return spec, nil
}

// deriveFanout reduces the per-namespace counts to a distinct count and the
// request share of the hottest namespace, discarding the namespace names.
func deriveFanout(c *counter) NamespaceFanout {
	f := NamespaceFanout{DistinctCount: len(c.counts)}
	if c.total == 0 {
		return f
	}
	top := 0
	for _, n := range c.counts {
		if n > top {
			top = n
		}
	}
	f.TopShare = float64(top) / float64(c.total)
	return f
}

// request is the parsed, still-identifying view of one access-log request. It is
// consumed entirely within Derive; only aggregate counts survive into the spec.
type request struct {
	op           OpType
	kind         string
	namespace    string
	limit        int
	labelKeys    []string
	isCollection bool
}

// parseRequest extracts a request from one log line, returning ok=false for lines
// that are not GETs against a recognizable Kubernetes-style resource path.
func parseRequest(line string) (request, bool) {
	method, target, ok := extractMethodPath(line)
	if !ok || method != "GET" {
		return request{}, false
	}
	rawPath, rawQuery, _ := strings.Cut(target, "?")
	seg := splitPath(rawPath)
	kind, namespace, shape, ok := classifyPath(seg)
	if !ok {
		return request{}, false
	}

	req := request{kind: kind, namespace: namespace}
	values, _ := url.ParseQuery(rawQuery)
	if v := values.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			req.limit = n
		}
	}
	req.labelKeys = selectorKeys(values.Get("labelSelector"))

	// KubeArchive's API (cmd/api/main.go) serves get-by-name and get-by-uid as
	// distinct point-lookup routes and reads only name/labelSelector/limit/continue
	// (no fieldSelector). Collection lists become label-selector lists when a
	// labelSelector is present, otherwise plain namespace/kind lists.
	switch shape {
	case shapeByName:
		req.op = OpGetByName
	case shapeByUID:
		req.op = OpOwnerUIDLookup
	case shapeCollection:
		req.isCollection = true
		if len(req.labelKeys) > 0 {
			req.op = OpLabelSelectorList
		} else {
			req.op = OpListByNamespaceKind
		}
	}
	return req, true
}

// extractMethodPath pulls the HTTP method and request target from one log line. It
// prefers KubeArchive's structured slog fields (`method=GET … path="/…"`, where the
// URL-encoded query is quoted) and falls back to the inline `GET /path` form. It
// returns ok=false when neither shape is present.
func extractMethodPath(line string) (method, target string, ok bool) {
	if m := methodField.FindStringSubmatch(line); m != nil {
		if p := pathField.FindStringSubmatch(line); p != nil {
			target = p[1]
			if target == "" {
				target = p[2]
			}
			return m[1], target, true
		}
	}
	if m := requestLine.FindStringSubmatch(line); m != nil {
		return m[1], m[2], true
	}
	return "", "", false
}

// splitPath returns the non-empty path segments.
func splitPath(path string) []string {
	parts := strings.Split(path, "/")
	seg := parts[:0]
	for _, p := range parts {
		if p != "" {
			seg = append(seg, p)
		}
	}
	return seg
}

// pathShape is the KubeArchive API route class a request path maps to.
type pathShape int

const (
	// shapeCollection is a list over a namespace/kind (or cluster-scoped kind).
	shapeCollection pathShape = iota
	// shapeByName is a single-object fetch by name.
	shapeByName
	// shapeByUID is a single-object fetch via the dedicated /uid/<uid> route.
	shapeByUID
)

// classifyPath recognizes KubeArchive's resource routes (cmd/api/main.go) and
// returns the kind, namespace (empty for cluster-scoped), and the route class. It
// accepts both grouped (/apis/<group>/<version>/…) and core (/api/<version>/…)
// forms. Unrecognized paths — including the /log subresource — return ok=false.
//
// Recognized shapes:
//
//	/apis/<group>/<version>/namespaces/<ns>/<kind>            → collection
//	/apis/<group>/<version>/namespaces/<ns>/<kind>/<name>     → by name
//	/apis/<group>/<version>/namespaces/<ns>/<kind>/uid/<uid>  → by uid
//	/apis/<group>/<version>/<kind>                            → collection (cluster-scoped)
//	(and the /api/<version>/… core-group equivalents)
func classifyPath(seg []string) (kind, namespace string, shape pathShape, ok bool) {
	var rest []string
	switch {
	case len(seg) >= 3 && seg[0] == "apis":
		rest = seg[3:] // drop apis/<group>/<version>
	case len(seg) >= 2 && seg[0] == "api":
		rest = seg[2:] // drop api/<version>
	default:
		return "", "", 0, false
	}

	if len(rest) >= 2 && rest[0] == "namespaces" {
		namespace = rest[1]
		rest = rest[2:]
	}
	switch {
	case len(rest) == 1:
		return rest[0], namespace, shapeCollection, true
	case len(rest) == 2:
		return rest[0], namespace, shapeByName, true
	case len(rest) == 3 && rest[1] == "uid":
		return rest[0], namespace, shapeByUID, true
	default:
		return "", "", 0, false
	}
}

// selectorKeys extracts the distinct label keys from a labelSelector value,
// discarding the values. It handles comma-separated equality and set-based terms
// (key=v, key==v, key!=v, key in (…), key notin (…), and bare key existence).
func selectorKeys(selector string) []string {
	if selector == "" {
		return nil
	}
	var keys []string
	seen := map[string]bool{}
	for term := range strings.SplitSeq(selector, ",") {
		key := selectorKey(term)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	return keys
}

// selectorKey returns the label key of a single selector term.
func selectorKey(term string) string {
	term = strings.TrimSpace(term)
	if term == "" {
		return ""
	}
	// Set-based: "key in (…)" / "key notin (…)".
	if i := strings.IndexAny(term, " "); i > 0 {
		if kw := strings.TrimSpace(term[i:]); strings.HasPrefix(kw, "in") || strings.HasPrefix(kw, "notin") {
			return strings.TrimSpace(term[:i])
		}
	}
	// Equality-based: key=, key==, key!=.
	if i := strings.IndexAny(term, "=!"); i > 0 {
		return strings.TrimSpace(term[:i])
	}
	// Bare existence: "key" or "!key".
	return strings.TrimPrefix(term, "!")
}
