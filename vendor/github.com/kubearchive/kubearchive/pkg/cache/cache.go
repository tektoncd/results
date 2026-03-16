// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package cache

import (
	"crypto/sha256"
	"sync"
	"time"
)

type cacheEntry struct {
	value      any
	expiration int64
}

type Cache struct {
	items map[[32]byte]cacheEntry
	mutex sync.RWMutex
}

func (c *Cache) Set(key string, value any, duration time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	hashedKey := sha256.Sum256([]byte(key))
	c.items[hashedKey] = cacheEntry{value: value, expiration: time.Now().Add(duration).Unix()}
}

func (c *Cache) Get(key string) any {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	hashedKey := sha256.Sum256([]byte(key))

	entry, ok := c.items[hashedKey]
	if !ok || time.Now().Unix() > entry.expiration {
		return nil
	}

	return entry.value
}

func New() *Cache {
	return &Cache{
		items: make(map[[32]byte]cacheEntry),
	}
}
