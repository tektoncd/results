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
	"testing"

	authzv1 "k8s.io/api/authorization/v1"
)

func TestBuildAuthzKey_GroupDelimiterCollision(t *testing.T) {
	k1 := buildAuthzKey("user", "uid", []string{"a,b"}, nil, "ns", resultsAPIGroup, "", "results", "", "", "get")
	k2 := buildAuthzKey("user", "uid", []string{"a", "b"}, nil, "ns", resultsAPIGroup, "", "results", "", "", "get")
	if k1 == k2 {
		t.Fatal("group lists [\"a,b\"] and [\"a\",\"b\"] must not share an authz cache key")
	}
}

func TestBuildAuthzKey_UserDelimiterCollision(t *testing.T) {
	k1 := buildAuthzKey("user|uid", "", nil, nil, "ns", resultsAPIGroup, "", "results", "", "", "get")
	k2 := buildAuthzKey("user", "uid", nil, nil, "ns", resultsAPIGroup, "", "results", "", "", "get")
	if k1 == k2 {
		t.Fatal("user/uid fields containing delimiters must not collide")
	}
}

func TestBuildAuthzKey_APIVersionDistinguishes(t *testing.T) {
	kEmpty := buildAuthzKey("u", "uid", nil, nil, "ns", "tekton.dev", "", "taskruns", "", "", "get")
	kV1 := buildAuthzKey("u", "uid", nil, nil, "ns", "tekton.dev", "v1", "taskruns", "", "", "get")
	kV1beta1 := buildAuthzKey("u", "uid", nil, nil, "ns", "tekton.dev", "v1beta1", "taskruns", "", "", "get")
	if kEmpty == kV1 {
		t.Fatal("empty API version must not collide with v1")
	}
	if kV1 == kV1beta1 {
		t.Fatal("v1 and v1beta1 must not share an authz cache key")
	}
}

func TestBuildAuthzKey_APIGroupDistinguishes(t *testing.T) {
	k1 := buildAuthzKey("u", "uid", nil, nil, "ns", "tekton.dev", "v1", "taskruns", "", "", "get")
	k2 := buildAuthzKey("u", "uid", nil, nil, "ns", "results.tekton.dev", "v1", "taskruns", "", "", "get")
	if k1 == k2 {
		t.Fatal("different API groups must not share an authz cache key")
	}
}

func TestBuildAuthzKey_NameAndSubresourceDistinguish(t *testing.T) {
	base := buildAuthzKey("u", "uid", nil, nil, "ns", "", "v1", "pods", "pod-a", "", "get")
	differentName := buildAuthzKey("u", "uid", nil, nil, "ns", "", "v1", "pods", "pod-b", "", "get")
	differentSubresource := buildAuthzKey("u", "uid", nil, nil, "ns", "", "v1", "pods", "pod-a", "log", "get")
	if base == differentName {
		t.Fatal("different resource names must not share an authz cache key")
	}
	if base == differentSubresource {
		t.Fatal("resource and subresource requests must not share an authz cache key")
	}
}

func TestBuildImpersonationAuthzKey_Distinct(t *testing.T) {
	k1 := buildImpersonationAuthzKey("admin", authzv1.ResourceAttributes{
		Resource: "users", Name: "alice", Verb: "impersonate",
	})
	k2 := buildImpersonationAuthzKey("admin", authzv1.ResourceAttributes{
		Resource: "users", Name: "bob", Verb: "impersonate",
	})
	if k1 == k2 {
		t.Fatal("impersonation targets must not share a cache key")
	}
}

func TestEncodeFields_OrderIndependent(t *testing.T) {
	a := encodeFields([]string{"b", "a"})
	b := encodeFields([]string{"a", "b"})
	if a != b {
		t.Fatalf("encodeFields should sort: %q vs %q", a, b)
	}
}

func TestEncodeExtra_PreservesValueOrder(t *testing.T) {
	k1 := encodeExtra(map[string]authzv1.ExtraValue{
		"scope": {"a", "b"},
	})
	k2 := encodeExtra(map[string]authzv1.ExtraValue{
		"scope": {"b", "a"},
	})
	if k1 == k2 {
		t.Fatal("Extra value order must affect encodeExtra so reversed lists do not share a cache key")
	}

	// Map key order alone must remain stable.
	k3 := encodeExtra(map[string]authzv1.ExtraValue{
		"b": {"1"},
		"a": {"1"},
	})
	k4 := encodeExtra(map[string]authzv1.ExtraValue{
		"a": {"1"},
		"b": {"1"},
	})
	if k3 != k4 {
		t.Fatalf("Extra map key iteration order must not change encoding: %q vs %q", k3, k4)
	}
}

func TestBuildAuthzKey_ExtraValueOrderDistinguishes(t *testing.T) {
	k1 := buildAuthzKey("u", "uid", nil, map[string]authzv1.ExtraValue{"scope": {"a", "b"}}, "ns", resultsAPIGroup, "", "results", "", "", "get")
	k2 := buildAuthzKey("u", "uid", nil, map[string]authzv1.ExtraValue{"scope": {"b", "a"}}, "ns", resultsAPIGroup, "", "results", "", "", "get")
	if k1 == k2 {
		t.Fatal("reversed Extra values must not share an authz cache key")
	}
}
