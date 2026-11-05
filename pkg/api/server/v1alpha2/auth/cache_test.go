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

	"github.com/jonboulle/clockwork"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCache_GetSet(t *testing.T) {
	c := NewCache(1 * time.Minute)

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
	clock := clockwork.NewFakeClock()
	c := newCacheWithClock(50*time.Millisecond, defaultCacheMaxSize, clock)

	c.Set("key1", "value1")
	if _, ok := c.Get("key1"); !ok {
		t.Fatal("expected cache hit before TTL")
	}

	clock.Advance(100 * time.Millisecond)

	if _, ok := c.Get("key1"); ok {
		t.Fatal("expected cache miss after TTL")
	}
}

func TestCache_GetRemovesExpiredEntry(t *testing.T) {
	clock := clockwork.NewFakeClock()
	c := newCacheWithClock(50*time.Millisecond, defaultCacheMaxSize, clock)

	c.Set("key1", "value1")
	if c.Len() != 1 {
		t.Fatalf("expected 1 entry, got %d", c.Len())
	}

	clock.Advance(100 * time.Millisecond)
	if _, ok := c.Get("key1"); ok {
		t.Fatal("expected cache miss after TTL")
	}
	if c.Len() != 0 {
		t.Fatalf("expected expired entry removed, Len=%d", c.Len())
	}
}

func TestCache_MaxSizeEvictsLeastRecentlyUsed(t *testing.T) {
	c := NewCacheWithMaxSize(time.Minute, 2)

	c.Set("a", 1)
	c.Set("b", 2)
	if _, ok := c.Get("a"); !ok {
		t.Fatal("expected a before eviction")
	}
	c.Set("c", 3)

	if c.Len() != 2 {
		t.Fatalf("Len=%d, want 2 at capacity", c.Len())
	}
	if c.MaxSize() != 2 {
		t.Fatalf("MaxSize=%d, want 2", c.MaxSize())
	}
	if _, ok := c.Get("b"); ok {
		t.Fatal("expected least recently used entry b to be evicted")
	}
	if _, ok := c.Get("a"); !ok {
		t.Fatal("expected recently used entry a to remain")
	}
	if _, ok := c.Get("c"); !ok {
		t.Fatal("expected newly added entry c to remain")
	}
}

func TestCache_NonPositiveTTLUsesDefault(t *testing.T) {
	c := NewCache(0)
	if c.ttl != DefaultPositiveCacheTTL {
		t.Fatalf("ttl = %v, want %v", c.ttl, DefaultPositiveCacheTTL)
	}

	c2 := NewCache(-time.Second)
	if c2.ttl != DefaultPositiveCacheTTL {
		t.Fatalf("ttl = %v, want %v", c2.ttl, DefaultPositiveCacheTTL)
	}
}

func TestWithCaching_DefaultsAndSizes(t *testing.T) {
	checker := NewRBAC(fake.NewSimpleClientset(), WithCaching(0, 0))

	if checker.authnCache.MaxSize() != defaultAuthnCacheMaxSize {
		t.Fatalf("authn cache size = %d, want %d", checker.authnCache.MaxSize(), defaultAuthnCacheMaxSize)
	}
	if checker.authzCache.MaxSize() != defaultAuthzCacheMaxSize {
		t.Fatalf("authz cache size = %d, want %d", checker.authzCache.MaxSize(), defaultAuthzCacheMaxSize)
	}
	if checker.authnCache.TTL() != DefaultPositiveCacheTTL {
		t.Fatalf("positive TTL = %v, want %v", checker.authnCache.TTL(), DefaultPositiveCacheTTL)
	}
	if checker.negativeCacheTTL != DefaultNegativeCacheTTL {
		t.Fatalf("negative TTL = %v, want %v", checker.negativeCacheTTL, DefaultNegativeCacheTTL)
	}
}

func TestCache_ConcurrentAccess(_ *testing.T) {
	c := NewCache(1 * time.Minute)

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
		in       string
		fallback time.Duration
		want     time.Duration
		wantErr  bool
	}{
		{in: "", fallback: DefaultPositiveCacheTTL, want: DefaultPositiveCacheTTL},
		{in: "", fallback: DefaultNegativeCacheTTL, want: DefaultNegativeCacheTTL},
		{in: "10m", fallback: DefaultPositiveCacheTTL, want: 10 * time.Minute},
		{in: "30s", fallback: DefaultNegativeCacheTTL, want: 30 * time.Second},
		{in: "not-a-duration", fallback: DefaultPositiveCacheTTL, want: DefaultPositiveCacheTTL, wantErr: true},
		{in: "0", fallback: DefaultNegativeCacheTTL, want: DefaultNegativeCacheTTL, wantErr: true},
		{in: "-1m", fallback: DefaultPositiveCacheTTL, want: DefaultPositiveCacheTTL, wantErr: true},
	}
	for _, tc := range tests {
		got, err := ParseCacheTTL(tc.in, tc.fallback)
		if got != tc.want {
			t.Errorf("ParseCacheTTL(%q)=%v, want %v", tc.in, got, tc.want)
		}
		if (err != nil) != tc.wantErr {
			t.Errorf("ParseCacheTTL(%q) error = %v, wantErr %v", tc.in, err, tc.wantErr)
		}
	}
}
