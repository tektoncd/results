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
	"time"

	k8scache "k8s.io/apimachinery/pkg/util/cache"
)

const (
	// DefaultPositiveCacheTTL is the default lifetime for successful authn and
	// allowed authz decisions.
	DefaultPositiveCacheTTL = 5 * time.Minute
	// DefaultNegativeCacheTTL is the default lifetime for unauthenticated and
	// denied decisions.
	DefaultNegativeCacheTTL = time.Minute

	defaultCacheMaxSize = 10000
)

// Clock provides the current time. It allows cache expiration to be tested
// without sleeping.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Cache is a thread-safe, bounded LRU cache with TTL-based expiration.
type Cache struct {
	entries *k8scache.LRUExpireCache
	ttl     time.Duration
	maxSize int
}

// NewCache creates a cache where every entry expires after ttl.
// Non-positive ttl values fall back to DefaultPositiveCacheTTL.
func NewCache(ttl time.Duration) *Cache {
	return NewCacheWithMaxSize(ttl, defaultCacheMaxSize)
}

// NewCacheWithMaxSize is like NewCache but sets an explicit entry cap.
func NewCacheWithMaxSize(ttl time.Duration, maxSize int) *Cache {
	return newCacheWithClock(ttl, maxSize, realClock{})
}

func newCacheWithClock(ttl time.Duration, maxSize int, clock Clock) *Cache {
	if ttl <= 0 {
		ttl = DefaultPositiveCacheTTL
	}
	if maxSize <= 0 {
		maxSize = defaultCacheMaxSize
	}
	return &Cache{
		entries: k8scache.NewLRUExpireCacheWithClock(maxSize, clock),
		ttl:     ttl,
		maxSize: maxSize,
	}
}

// Get retrieves a cached value. Returns (value, true) if found and not
// expired, or (nil, false) otherwise.
func (c *Cache) Get(key string) (any, bool) {
	return c.entries.Get(key)
}

// Set stores a value with the configured TTL.
func (c *Cache) Set(key string, value any) {
	c.SetWithTTL(key, value, c.ttl)
}

// SetWithTTL stores a value that expires after ttl. Non-positive ttl skips
// caching so callers can refuse to retain already-expired decisions.
// At capacity, the least recently used entry is evicted.
func (c *Cache) SetWithTTL(key string, value any, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	c.entries.Add(key, value, ttl)
}

// TTL returns the default entry lifetime configured for this cache.
func (c *Cache) TTL() time.Duration {
	return c.ttl
}

// Len returns the number of unexpired entries currently stored. Intended for tests.
func (c *Cache) Len() int {
	return len(c.entries.Keys())
}

// MaxSize returns the configured entry cap. Intended for tests.
func (c *Cache) MaxSize() int {
	return c.maxSize
}

// Keys returns a snapshot of unexpired cache keys. Intended for tests.
func (c *Cache) Keys() []string {
	cacheKeys := c.entries.Keys()
	keys := make([]string, 0, len(cacheKeys))
	for _, key := range cacheKeys {
		keys = append(keys, key.(string))
	}
	return keys
}
