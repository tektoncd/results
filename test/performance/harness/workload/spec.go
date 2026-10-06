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

// Package workload describes the read workload the query benchmark replays. A
// QueryMixSpec is an anonymized statistical description of a real query workload
// derived from KubeArchive deployment access logs: relative frequencies of
// operation types, the page-size distribution, label-selector shapes, and
// namespace fan-out. It intentionally records only kinds, label keys, and numeric
// distributions — never raw namespaces, tokens, hostnames, label values, or UIDs —
// so the checked-in spec leaks no identifiers from the source deployment.
//
// The spec is produced offline by Derive (see derive.go, wired to `bench derive`)
// and consumed by the query driver to weight the request stream so the benchmark
// reflects real-world access patterns rather than a hand-guessed mix.
package workload

import (
	_ "embed"
	"fmt"
	"io"
	"sort"

	"sigs.k8s.io/yaml"
)

// SchemaVersion identifies the spec format; bump on any breaking field change.
const SchemaVersion = "1"

// OpType enumerates the query operation classes the driver distinguishes. The
// values mirror the access patterns visible in KubeArchive logs.
type OpType string

const (
	// OpListByNamespaceKind is a plain namespaced collection list (no selector).
	OpListByNamespaceKind OpType = "list_by_namespace_kind"
	// OpLabelSelectorList is a namespaced collection list carrying a labelSelector.
	OpLabelSelectorList OpType = "label_selector_list"
	// OpGetByName is a single-object fetch by name.
	OpGetByName OpType = "get_by_name"
	// OpOwnerUIDLookup is a lookup keyed by owner/uid (uid label or field selector).
	OpOwnerUIDLookup OpType = "owner_uid_lookup"
)

// QueryMixSpec is the anonymized description of a query workload.
type QueryMixSpec struct {
	SchemaVersion   string            `json:"schema_version"`
	Source          string            `json:"source,omitempty"`
	SampleSize      int               `json:"sample_size"`
	Operations      []OperationWeight `json:"operations"`
	PageSizes       []PageSizeWeight  `json:"page_sizes"`
	LabelSelector   LabelSelectorMix  `json:"label_selector"`
	NamespaceFanout NamespaceFanout   `json:"namespace_fanout"`
}

// OperationWeight is the observed frequency of one operation class, optionally
// broken down by resource kind (e.g. pipelineruns, taskruns).
type OperationWeight struct {
	Type   OpType  `json:"type"`
	Kind   string  `json:"kind,omitempty"`
	Count  int     `json:"count"`
	Weight float64 `json:"weight"`
}

// PageSizeWeight is the observed frequency of a requested page size (limit=).
type PageSizeWeight struct {
	Limit  int     `json:"limit"`
	Count  int     `json:"count"`
	Weight float64 `json:"weight"`
}

// LabelSelectorMix summarizes how list requests use label selectors. Only label
// keys are recorded — keys are schema (safe to keep), values are identifiers and
// are never stored.
type LabelSelectorMix struct {
	// Fraction is the share of collection-list requests carrying a selector.
	Fraction float64          `json:"fraction"`
	Keys     []LabelKeyWeight `json:"keys,omitempty"`
}

// LabelKeyWeight is the observed frequency of a single label-selector key.
type LabelKeyWeight struct {
	Key    string  `json:"key"`
	Count  int     `json:"count"`
	Weight float64 `json:"weight"`
}

// NamespaceFanout captures how widely requests spread across namespaces without
// recording any namespace name. DistinctCount is the number of distinct
// namespaces seen; TopShare is the request share of the single hottest namespace
// (1.0 = all traffic to one namespace, → 0 = perfectly spread).
type NamespaceFanout struct {
	DistinctCount int     `json:"distinct_count"`
	TopShare      float64 `json:"top_share"`
}

//go:embed querymix.yaml
var defaultSpecBytes []byte

// Default returns the committed query-mix spec embedded in the binary. The query
// driver uses it when no --query-spec override is supplied.
func Default() (*QueryMixSpec, error) {
	return Load(defaultSpecBytes)
}

// Load decodes a spec from YAML (JSON is valid YAML, so both are accepted).
func Load(data []byte) (*QueryMixSpec, error) {
	spec := &QueryMixSpec{}
	if err := yaml.Unmarshal(data, spec); err != nil {
		return nil, fmt.Errorf("decoding query-mix spec: %w", err)
	}
	return spec, nil
}

// Write emits the spec as YAML with a trailing newline.
func Write(w io.Writer, spec *QueryMixSpec) error {
	data, err := yaml.Marshal(spec)
	if err != nil {
		return fmt.Errorf("encoding query-mix spec: %w", err)
	}
	_, err = w.Write(data)
	return err
}

// Validate checks the spec is internally consistent enough to drive a run.
func (s *QueryMixSpec) Validate() error {
	if s.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported spec schema version %q (want %q)", s.SchemaVersion, SchemaVersion)
	}
	if len(s.Operations) == 0 {
		return fmt.Errorf("spec has no operations")
	}
	return nil
}

// counter accumulates observations for one categorical distribution while Derive
// scans the log, then renders them into weighted, deterministically ordered
// slices.
type counter struct {
	counts map[string]int
	total  int
}

func newCounter() *counter { return &counter{counts: map[string]int{}} }

func (c *counter) add(key string) {
	c.counts[key]++
	c.total++
}

// keysByFrequency returns the observed keys ordered by descending count, ties
// broken lexicographically, so a given log always yields the same spec ordering.
func (c *counter) keysByFrequency() []string {
	keys := make([]string, 0, len(c.counts))
	for k := range c.counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if c.counts[keys[i]] != c.counts[keys[j]] {
			return c.counts[keys[i]] > c.counts[keys[j]]
		}
		return keys[i] < keys[j]
	})
	return keys
}

// weight returns count/total, or 0 when nothing was observed.
func (c *counter) weight(count int) float64 {
	if c.total == 0 {
		return 0
	}
	return float64(count) / float64(c.total)
}
