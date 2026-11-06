// Copyright 2026 The Tekton Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auth

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	authzv1 "k8s.io/api/authorization/v1"
)

// resultsAPIGroup is the default API group used in SubjectAccessReview
// resource attributes for the Results gRPC path.
const resultsAPIGroup = "results.tekton.dev"

// resultsAPIVersion is the default API version for the Results gRPC path.
// Legacy Results SARs leave Version empty; the K8s-style HTTP path sets a
// non-empty version via WithResourceAttributes.
const resultsAPIVersion = ""

// NewCachingChecker enables in-memory TokenReview and SubjectAccessReview
// caching on the given RBAC checker. Caching is applied inside
// authenticate/authorize/checkImpersonation so Check stays single-sourced.
// The returned value is the same RBAC instance (which already implements Checker).
//
// The K8s-style HTTP auth middleware in pkg/api/server/k8s shares this cache
// by wrapping the same Checker and setting resource details via
// WithResourceAttributes.
func NewCachingChecker(inner *RBAC, authnTTL, authzTTL time.Duration) *RBAC {
	inner.authnCache = NewCache(authnTTL)
	inner.authzCache = NewCache(authzTTL)
	return inner
}

// CacheConfig holds AUTH_CACHE_* settings used to construct a Checker.
// AuthnTTL/AuthzTTL should already be parsed (see ParseCacheTTL); non-positive
// values are still sanitized inside NewCache.
type CacheConfig struct {
	Enabled  bool
	AuthnTTL time.Duration
	AuthzTTL time.Duration
}

// NewCheckerFromConfig returns an RBAC checker, optionally wrapping it with
// caches when cfg.Enabled is true. This is the shared factory for the gRPC
// Results path and the K8s-style HTTP middleware path.
func NewCheckerFromConfig(inner *RBAC, cfg CacheConfig) *RBAC {
	if !cfg.Enabled {
		return inner
	}
	return NewCachingChecker(inner, cfg.AuthnTTL, cfg.AuthzTTL)
}

// ParseCacheTTL parses a duration config value. Empty, invalid, or non-positive
// values fall back to the default TTL (5m) so callers never construct a cache
// with ttl <= 0 (time.NewTicker panics).
func ParseCacheTTL(value string) time.Duration {
	if value == "" {
		return defaultCacheTTL
	}
	ttl, err := time.ParseDuration(value)
	if err != nil || ttl <= 0 {
		return defaultCacheTTL
	}
	return ttl
}

// hashString returns the hex-encoded SHA-256 hash of s.
// Used for cache keys so raw tokens are never stored in memory.
func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h)
}

// encodeField length-prefixes a string so delimiters inside values cannot
// collide with separators used when joining fields.
func encodeField(s string) string {
	return strconv.Itoa(len(s)) + ":" + s
}

// encodeFields encodes a list with a count prefix and length-prefixed items.
func encodeFields(values []string) string {
	sorted := make([]string, len(values))
	copy(sorted, values)
	sort.Strings(sorted)
	var b strings.Builder
	b.WriteString(strconv.Itoa(len(sorted)))
	for _, v := range sorted {
		b.WriteByte('|')
		b.WriteString(encodeField(v))
	}
	return b.String()
}

// buildAuthzKey creates a deterministic cache key from the authorization
// parameters that are sent to SubjectAccessReview. apiGroup and apiVersion are
// the ResourceAttributes group/version (not the user's group list). Fields are
// length-prefixed so values containing commas/pipes cannot collide.
func buildAuthzKey(user, uid string, groups []string, extra map[string]authzv1.ExtraValue, namespace, apiGroup, apiVersion, resource, name, subresource, verb string) string {
	raw := strings.Join([]string{
		encodeField(user),
		encodeField(uid),
		encodeFields(groups),
		encodeExtra(extra),
		encodeField(namespace),
		encodeField(apiGroup),
		encodeField(apiVersion),
		encodeField(resource),
		encodeField(name),
		encodeField(subresource),
		encodeField(verb),
	}, "|")
	return hashString(raw)
}

// buildImpersonationAuthzKey keys an impersonation SubjectAccessReview.
func buildImpersonationAuthzKey(requester string, ra authzv1.ResourceAttributes) string {
	raw := strings.Join([]string{
		encodeField("impersonate"),
		encodeField(requester),
		encodeField(ra.Namespace),
		encodeField(ra.Group),
		encodeField(ra.Resource),
		encodeField(ra.Subresource),
		encodeField(ra.Name),
		encodeField(ra.Verb),
	}, "|")
	return hashString(raw)
}

// encodeExtra produces a stable, length-prefixed form of SAR Extra attributes.
func encodeExtra(extra map[string]authzv1.ExtraValue) string {
	if len(extra) == 0 {
		return encodeField("")
	}
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(strconv.Itoa(len(keys)))
	for _, k := range keys {
		vals := append([]string(nil), extra[k]...)
		b.WriteByte('|')
		b.WriteString(encodeField(k))
		b.WriteByte('=')
		b.WriteString(encodeFields(vals))
	}
	return b.String()
}
