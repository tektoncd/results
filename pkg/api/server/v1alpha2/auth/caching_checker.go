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

const (
	// Authentication has one entry per token and normally has lower cardinality.
	defaultAuthnCacheMaxSize = 10_000
	// Authorization includes identity and resource attributes, so it needs more room.
	defaultAuthzCacheMaxSize = 100_000
)

type cacheOptions struct {
	positiveTTL time.Duration
	negativeTTL time.Duration
}

// WithCaching enables in-memory TokenReview and SubjectAccessReview caching.
// Authentication is capped at 10,000 entries and authorization at 100,000.
func WithCaching(positiveTTL, negativeTTL time.Duration) Option {
	return func(r *RBAC) {
		r.cacheOptions = &cacheOptions{
			positiveTTL: positiveTTL,
			negativeTTL: negativeTTL,
		}
	}
}

// WithClock sets the clock used for cache and JWT expiration. It is primarily
// useful for deterministic tests.
func WithClock(clock Clock) Option {
	return func(r *RBAC) {
		if clock != nil {
			r.clock = clock
		}
	}
}

func (r *RBAC) initializeCaches() {
	if r.cacheOptions == nil {
		return
	}
	positiveTTL := r.cacheOptions.positiveTTL
	if positiveTTL <= 0 {
		positiveTTL = DefaultPositiveCacheTTL
	}
	negativeTTL := r.cacheOptions.negativeTTL
	if negativeTTL <= 0 {
		negativeTTL = DefaultNegativeCacheTTL
	}
	r.authnCache = newCacheWithClock(positiveTTL, defaultAuthnCacheMaxSize, r.clock)
	r.authzCache = newCacheWithClock(positiveTTL, defaultAuthzCacheMaxSize, r.clock)
	r.negativeCacheTTL = negativeTTL
}

// ParseCacheTTL parses a duration config value once. Empty values use fallback;
// invalid and non-positive values return fallback along with an explanatory error.
func ParseCacheTTL(value string, fallback time.Duration) (time.Duration, error) {
	if value == "" {
		return fallback, nil
	}
	ttl, err := time.ParseDuration(value)
	if err != nil {
		return fallback, fmt.Errorf("invalid duration %q: %w", value, err)
	}
	if ttl <= 0 {
		return fallback, fmt.Errorf("non-positive duration %q", value)
	}
	return ttl, nil
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
// Values are sorted so unordered sets (e.g. group membership) share one key.
func encodeFields(values []string) string {
	sorted := make([]string, len(values))
	copy(sorted, values)
	sort.Strings(sorted)
	return encodeFieldsInOrder(sorted)
}

// encodeFieldsInOrder encodes a list preserving value order. Used for Extra
// attribute values where authorizers may treat order as significant.
func encodeFieldsInOrder(values []string) string {
	var b strings.Builder
	b.WriteString(strconv.Itoa(len(values)))
	for _, v := range values {
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
		// Preserve Extra value order; only map keys are sorted for stability.
		vals := append([]string(nil), extra[k]...)
		b.WriteByte('|')
		b.WriteString(encodeField(k))
		b.WriteByte('=')
		b.WriteString(encodeFieldsInOrder(vals))
	}
	return b.String()
}
