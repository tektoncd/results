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

package config

import (
	"testing"
)

func TestAuthCacheDefaults(t *testing.T) {
	v := newViper()

	if got := v.GetBool("AUTH_CACHE_ENABLED"); got {
		t.Fatalf("AUTH_CACHE_ENABLED default = %v, want false", got)
	}
	if got := v.GetString("AUTH_CACHE_POSITIVE_TTL"); got != "5m" {
		t.Fatalf("AUTH_CACHE_POSITIVE_TTL default = %q, want 5m", got)
	}
	if got := v.GetString("AUTH_CACHE_NEGATIVE_TTL"); got != "1m" {
		t.Fatalf("AUTH_CACHE_NEGATIVE_TTL default = %q, want 1m", got)
	}
}

func TestAuthCacheEnvOverride(t *testing.T) {
	v := newViper()
	v.AutomaticEnv()

	t.Setenv("AUTH_CACHE_ENABLED", "true")
	t.Setenv("AUTH_CACHE_POSITIVE_TTL", "1m")
	t.Setenv("AUTH_CACHE_NEGATIVE_TTL", "2m")

	if got := v.GetBool("AUTH_CACHE_ENABLED"); !got {
		t.Fatalf("AUTH_CACHE_ENABLED = %v, want true from env", got)
	}
	if got := v.GetString("AUTH_CACHE_POSITIVE_TTL"); got != "1m" {
		t.Fatalf("AUTH_CACHE_POSITIVE_TTL = %q, want 1m", got)
	}
	if got := v.GetString("AUTH_CACHE_NEGATIVE_TTL"); got != "2m" {
		t.Fatalf("AUTH_CACHE_NEGATIVE_TTL = %q, want 2m", got)
	}
}
