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

package k8s_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tektoncd/results/pkg/api/server/k8s"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth"
	authnv1 "k8s.io/api/authentication/v1"
	authzv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	test "k8s.io/client-go/testing"
)

func TestAuth_MiddlewareUsesSharedCache(t *testing.T) {
	var tokenReviewCount atomic.Int32
	var sarCount atomic.Int32
	var lastVersion string

	k8sClient := fake.NewSimpleClientset()
	k8sClient.PrependReactor("create", "tokenreviews", func(action test.Action) (bool, runtime.Object, error) {
		tokenReviewCount.Add(1)
		tr := action.(test.CreateActionImpl).Object.(*authnv1.TokenReview)
		tr.Status = authnv1.TokenReviewStatus{
			Authenticated: true,
			User:          authnv1.UserInfo{Username: "test-user"},
		}
		return true, tr, nil
	})
	k8sClient.PrependReactor("create", "subjectaccessreviews", func(action test.Action) (bool, runtime.Object, error) {
		sarCount.Add(1)
		sar := action.(test.CreateActionImpl).Object.(*authzv1.SubjectAccessReview)
		if sar.Spec.ResourceAttributes != nil {
			lastVersion = sar.Spec.ResourceAttributes.Version
		}
		sar.Status = authzv1.SubjectAccessReviewStatus{Allowed: true}
		return true, sar, nil
	})

	checker := auth.NewCachingChecker(auth.NewRBAC(k8sClient), 5*time.Minute, 5*time.Minute)
	t.Cleanup(checker.Stop)
	httpAuth := k8s.NewAuth(checker)

	attrs := k8s.ResourceAttrs{
		Namespace: "default",
		Group:     "tekton.dev",
		Version:   "v1",
		Resource:  "pipelineruns",
		Verb:      "get",
	}
	handler := httpAuth.Handler(attrs, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/apis/tekton.dev/v1/namespaces/default/pipelineruns/x", nil)
	req.Header.Set("Authorization", "Bearer test-token")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("first request status=%d body=%s", rr.Code, rr.Body.String())
	}
	if lastVersion != "v1" {
		t.Fatalf("SAR Version=%q, want v1", lastVersion)
	}
	if tokenReviewCount.Load() != 1 || sarCount.Load() != 1 {
		t.Fatalf("expected 1 TokenReview and 1 SAR, got %d / %d", tokenReviewCount.Load(), sarCount.Load())
	}

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("cached request status=%d", rr.Code)
	}
	if tokenReviewCount.Load() != 1 || sarCount.Load() != 1 {
		t.Fatalf("expected cache hit (still 1/1), got %d / %d", tokenReviewCount.Load(), sarCount.Load())
	}
}

func TestAuth_APIVersionDistinguishesCacheKeys(t *testing.T) {
	var sarCount atomic.Int32

	k8sClient := fake.NewSimpleClientset()
	k8sClient.PrependReactor("create", "tokenreviews", func(action test.Action) (bool, runtime.Object, error) {
		tr := action.(test.CreateActionImpl).Object.(*authnv1.TokenReview)
		tr.Status = authnv1.TokenReviewStatus{
			Authenticated: true,
			User:          authnv1.UserInfo{Username: "test-user"},
		}
		return true, tr, nil
	})
	k8sClient.PrependReactor("create", "subjectaccessreviews", func(action test.Action) (bool, runtime.Object, error) {
		sarCount.Add(1)
		sar := action.(test.CreateActionImpl).Object.(*authzv1.SubjectAccessReview)
		sar.Status = authzv1.SubjectAccessReviewStatus{Allowed: true}
		return true, sar, nil
	})

	checker := auth.NewCachingChecker(auth.NewRBAC(k8sClient), 5*time.Minute, 5*time.Minute)
	t.Cleanup(checker.Stop)
	httpAuth := k8s.NewAuth(checker)

	base := k8s.ResourceAttrs{
		Namespace: "default",
		Group:     "tekton.dev",
		Resource:  "taskruns",
		Verb:      "list",
	}
	for _, version := range []string{"v1", "v1beta1"} {
		attrs := base
		attrs.Version = version
		handler := httpAuth.Handler(attrs, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("version %s status=%d", version, rr.Code)
		}
	}
	if sarCount.Load() != 2 {
		t.Fatalf("expected distinct SAR cache keys per API version (2 calls), got %d", sarCount.Load())
	}
}

func TestAuth_PreservesCoreAPIGroup(t *testing.T) {
	var gotGroup, gotVersion, gotName, gotSubresource string

	k8sClient := fake.NewSimpleClientset()
	k8sClient.PrependReactor("create", "tokenreviews", func(action test.Action) (bool, runtime.Object, error) {
		tr := action.(test.CreateActionImpl).Object.(*authnv1.TokenReview)
		tr.Status = authnv1.TokenReviewStatus{
			Authenticated: true,
			User:          authnv1.UserInfo{Username: "test-user"},
		}
		return true, tr, nil
	})
	k8sClient.PrependReactor("create", "subjectaccessreviews", func(action test.Action) (bool, runtime.Object, error) {
		sar := action.(test.CreateActionImpl).Object.(*authzv1.SubjectAccessReview)
		gotGroup = sar.Spec.ResourceAttributes.Group
		gotVersion = sar.Spec.ResourceAttributes.Version
		gotName = sar.Spec.ResourceAttributes.Name
		gotSubresource = sar.Spec.ResourceAttributes.Subresource
		sar.Status = authzv1.SubjectAccessReviewStatus{Allowed: true}
		return true, sar, nil
	})

	httpAuth := k8s.NewAuth(auth.NewRBAC(k8sClient))
	handler := httpAuth.Handler(k8s.ResourceAttrs{
		Namespace:   "default",
		Group:       "",
		Version:     "v1",
		Resource:    "pods",
		Name:        "example",
		Subresource: "log",
		Verb:        "get",
	}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/pods/example/log", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotGroup != "" || gotVersion != "v1" {
		t.Fatalf("SAR group/version=%q/%q, want empty/v1", gotGroup, gotVersion)
	}
	if gotName != "example" || gotSubresource != "log" {
		t.Fatalf("SAR name/subresource=%q/%q, want example/log", gotName, gotSubresource)
	}
}

func TestAuth_Unauthorized(t *testing.T) {
	k8sClient := fake.NewSimpleClientset()
	k8sClient.PrependReactor("create", "tokenreviews", func(action test.Action) (bool, runtime.Object, error) {
		tr := action.(test.CreateActionImpl).Object.(*authnv1.TokenReview)
		tr.Status = authnv1.TokenReviewStatus{Authenticated: false}
		return true, tr, nil
	})

	httpAuth := k8s.NewAuth(auth.NewRBAC(k8sClient))
	handler := httpAuth.Handler(k8s.ResourceAttrs{
		Namespace: "default",
		Group:     "tekton.dev",
		Version:   "v1",
		Resource:  "pipelineruns",
		Verb:      "get",
	}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer bad")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", rr.Code)
	}
	if got := rr.Body.String(); got != "Unauthorized\n" {
		t.Fatalf("body=%q, want fixed unauthorized response", got)
	}
}
