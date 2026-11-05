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
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestCache_GetSet(t *testing.T) {
	c := NewCache(1 * time.Minute)
	t.Cleanup(c.Stop)

	if _, ok := c.Get("key1"); ok {
		t.Fatal("expected cache miss on empty cache")
	}

	c.Set("key1", "value1")
	val, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected cache hit after Set")
	}
	if val.(string) != "value1" {
		t.Fatalf("got %v, want value1", val)
	}
}

func TestCache_Expiry(t *testing.T) {
	c := NewCache(50 * time.Millisecond)
	t.Cleanup(c.Stop)

	c.Set("key1", "value1")
	if _, ok := c.Get("key1"); !ok {
		t.Fatal("expected cache hit before TTL")
	}

	time.Sleep(100 * time.Millisecond)

	if _, ok := c.Get("key1"); ok {
		t.Fatal("expected cache miss after TTL")
	}
}

func TestCache_CleanupTickerPurgesExpiredEntries(t *testing.T) {
	c := NewCache(50 * time.Millisecond)
	t.Cleanup(c.Stop)

	c.Set("key1", "value1")
	if c.Len() != 1 {
		t.Fatalf("expected 1 entry, got %d", c.Len())
	}

	// Wait for TTL expiry plus at least one background cleanup tick.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if c.Len() == 0 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("expected ticker to purge expired entry, Len=%d", c.Len())
}

func TestCache_MaxSizeEvicts(t *testing.T) {
	c := NewCacheWithMaxSize(time.Minute, 2)
	t.Cleanup(c.Stop)

	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)

	if c.Len() != 2 {
		t.Fatalf("Len=%d, want 2 at capacity", c.Len())
	}
	if c.MaxSize() != 2 {
		t.Fatalf("MaxSize=%d, want 2", c.MaxSize())
	}
	c.Set("a", 10)
	if c.Len() != 2 {
		t.Fatalf("Len after update=%d, want 2", c.Len())
	}
}

func TestCache_NonPositiveTTLUsesDefault(t *testing.T) {
	c := NewCache(0)
	t.Cleanup(c.Stop)
	if c.ttl != defaultCacheTTL {
		t.Fatalf("ttl = %v, want %v", c.ttl, defaultCacheTTL)
	}

	c2 := NewCache(-time.Second)
	t.Cleanup(c2.Stop)
	if c2.ttl != defaultCacheTTL {
		t.Fatalf("ttl = %v, want %v", c2.ttl, defaultCacheTTL)
	}
}

func TestCache_StopIdempotent(_ *testing.T) {
	c := NewCache(time.Minute)
	c.Stop()
	c.Stop()
}

func TestCache_ConcurrentAccess(t *testing.T) {
	c := NewCache(1 * time.Minute)
	t.Cleanup(c.Stop)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			c.Set(fmt.Sprintf("key%d", i), i)
		}(i)
		go func(i int) {
			defer wg.Done()
			c.Get(fmt.Sprintf("key%d", i))
		}(i)
	}
	wg.Wait()
}

func TestParseCacheTTL(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{in: "", want: defaultCacheTTL},
		{in: "10m", want: 10 * time.Minute},
		{in: "30s", want: 30 * time.Second},
		{in: "not-a-duration", want: defaultCacheTTL},
		{in: "0", want: defaultCacheTTL},
		{in: "-1m", want: defaultCacheTTL},
	}
	for _, tc := range tests {
		if got := ParseCacheTTL(tc.in); got != tc.want {
			t.Errorf("ParseCacheTTL(%q)=%v, want %v", tc.in, got, tc.want)
		}
	}
}
