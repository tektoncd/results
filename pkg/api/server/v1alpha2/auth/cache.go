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
	"sync"
	"time"
)

const (
	defaultCacheTTL     = 5 * time.Minute
	defaultCacheMaxSize = 10000
)

// Cache is a thread-safe in-memory cache with TTL-based expiration and a
// maximum entry count to bound memory under attacker-controlled keys.
type Cache struct {
	mu       sync.RWMutex
	entries  map[string]cacheEntry
	ttl      time.Duration
	maxSize  int
	done     chan struct{}
	stopOnce sync.Once
}

type cacheEntry struct {
	value     any
	expiresAt time.Time
}

// NewCache creates a cache where every entry expires after ttl.
// Non-positive ttl values fall back to 5m so time.NewTicker never panics.
// It starts a background goroutine that periodically removes expired entries.
// Call Stop when the cache is no longer needed.
func NewCache(ttl time.Duration) *Cache {
	return NewCacheWithMaxSize(ttl, defaultCacheMaxSize)
}

// NewCacheWithMaxSize is like NewCache but sets an explicit entry cap.
func NewCacheWithMaxSize(ttl time.Duration, maxSize int) *Cache {
	if ttl <= 0 {
		ttl = defaultCacheTTL
	}
	if maxSize <= 0 {
		maxSize = defaultCacheMaxSize
	}
	c := &Cache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
		maxSize: maxSize,
		done:    make(chan struct{}),
	}
	go c.cleanup()
	return c
}

// Get retrieves a cached value. Returns (value, true) if found and not
// expired, or (nil, false) otherwise.
func (c *Cache) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.value, true
}

// Set stores a value with the configured TTL. If the cache is at capacity,
// expired entries are purged first; if still full, one existing entry is
// evicted so memory stays bounded under unique-key floods.
func (c *Cache) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.entries[key]; !exists && len(c.entries) >= c.maxSize {
		c.purgeExpiredLocked(time.Now())
		if len(c.entries) >= c.maxSize {
			c.evictOneLocked()
		}
	}

	c.entries[key] = cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// Len returns the number of entries currently stored (including expired
// ones that have not yet been purged). Intended for tests.
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// MaxSize returns the configured entry cap. Intended for tests.
func (c *Cache) MaxSize() int {
	return c.maxSize
}

// Keys returns a snapshot of cache keys. Intended for tests.
func (c *Cache) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	keys := make([]string, 0, len(c.entries))
	for k := range c.entries {
		keys = append(keys, k)
	}
	return keys
}

// Stop shuts down the background cleanup goroutine. Safe to call multiple times.
func (c *Cache) Stop() {
	c.stopOnce.Do(func() {
		close(c.done)
	})
}

// cleanup periodically removes expired entries to prevent memory growth.
func (c *Cache) cleanup() {
	ticker := time.NewTicker(c.ttl)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.mu.Lock()
			c.purgeExpiredLocked(time.Now())
			c.mu.Unlock()
		case <-c.done:
			return
		}
	}
}

func (c *Cache) purgeExpiredLocked(now time.Time) {
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
}

// evictOneLocked deletes one arbitrary entry. Caller must hold c.mu.
func (c *Cache) evictOneLocked() {
	for key := range c.entries {
		delete(c.entries, key)
		return
	}
}
