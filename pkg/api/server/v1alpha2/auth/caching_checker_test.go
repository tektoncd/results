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

package auth_test

import (
	"context"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	authnv1 "k8s.io/api/authentication/v1"
	authzv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	test "k8s.io/client-go/testing"
)

func TestCachingChecker_CacheHit(t *testing.T) {
	var tokenReviewCount atomic.Int32
	var sarCount atomic.Int32

	k8s := fake.NewSimpleClientset()
	k8s.PrependReactor("create", "tokenreviews", func(action test.Action) (bool, runtime.Object, error) {
		tokenReviewCount.Add(1)
		tr := action.(test.CreateActionImpl).Object.(*authnv1.TokenReview)
		tr.Status = authnv1.TokenReviewStatus{
			Authenticated: true,
			User: authnv1.UserInfo{
				Username: "test-user",
				UID:      "uid-1",
				Groups:   []string{"test-group"},
			},
		}
		return true, tr, nil
	})
	k8s.PrependReactor("create", "subjectaccessreviews", func(action test.Action) (bool, runtime.Object, error) {
		sarCount.Add(1)
		sar := action.(test.CreateActionImpl).Object.(*authzv1.SubjectAccessReview)
		sar.Status = authzv1.SubjectAccessReviewStatus{Allowed: true}
		return true, sar, nil
	})

	checker := auth.NewCachingChecker(auth.NewRBAC(k8s), 5*time.Minute, 5*time.Minute)
	t.Cleanup(checker.Stop)

	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("authorization", "Bearer test-token"))

	if err := checker.Check(ctx, "default", "results", "get"); err != nil {
		t.Fatalf("first Check failed: %v", err)
	}
	if tokenReviewCount.Load() != 1 || sarCount.Load() != 1 {
		t.Fatalf("expected 1 TokenReview and 1 SAR, got %d / %d", tokenReviewCount.Load(), sarCount.Load())
	}

	if err := checker.Check(ctx, "default", "results", "get"); err != nil {
		t.Fatalf("second Check failed: %v", err)
	}
	if tokenReviewCount.Load() != 1 || sarCount.Load() != 1 {
		t.Fatalf("expected cache hit (still 1/1), got %d / %d", tokenReviewCount.Load(), sarCount.Load())
	}

	if err := checker.Check(ctx, "default", "records", "get"); err != nil {
		t.Fatalf("third Check failed: %v", err)
	}
	if sarCount.Load() != 2 {
		t.Fatalf("expected 2 SAR calls (new resource), got %d", sarCount.Load())
	}
}

func TestCachingChecker_DeniedResultIsCached(t *testing.T) {
	var sarCount atomic.Int32

	k8s := fake.NewSimpleClientset()
	k8s.PrependReactor("create", "tokenreviews", func(action test.Action) (bool, runtime.Object, error) {
		tr := action.(test.CreateActionImpl).Object.(*authnv1.TokenReview)
		tr.Status = authnv1.TokenReviewStatus{
			Authenticated: true,
			User:          authnv1.UserInfo{Username: "denied-user"},
		}
		return true, tr, nil
	})
	k8s.PrependReactor("create", "subjectaccessreviews", func(action test.Action) (bool, runtime.Object, error) {
		sarCount.Add(1)
		sar := action.(test.CreateActionImpl).Object.(*authzv1.SubjectAccessReview)
		sar.Status = authzv1.SubjectAccessReviewStatus{Allowed: false}
		return true, sar, nil
	})

	checker := auth.NewCachingChecker(auth.NewRBAC(k8s), 5*time.Minute, 5*time.Minute)
	t.Cleanup(checker.Stop)

	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("authorization", "Bearer denied-token"))

	for i := 0; i < 2; i++ {
		if err := checker.Check(ctx, "default", "results", "get"); status.Code(err) != codes.Unauthenticated {
			t.Fatalf("Check = %v, want Unauthenticated", err)
		}
	}
	if sarCount.Load() != 1 {
		t.Fatalf("expected denied result cached (1 SAR), got %d", sarCount.Load())
	}
}

func TestCachingChecker_UnauthenticatedTokenIsCached(t *testing.T) {
	var tokenReviewCount atomic.Int32

	k8s := fake.NewSimpleClientset()
	k8s.PrependReactor("create", "tokenreviews", func(action test.Action) (bool, runtime.Object, error) {
		tokenReviewCount.Add(1)
		tr := action.(test.CreateActionImpl).Object.(*authnv1.TokenReview)
		tr.Status = authnv1.TokenReviewStatus{Authenticated: false}
		return true, tr, nil
	})

	checker := auth.NewCachingChecker(auth.NewRBAC(k8s), 5*time.Minute, 5*time.Minute)
	t.Cleanup(checker.Stop)

	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("authorization", "Bearer bad-token"))

	_ = checker.Check(ctx, "default", "results", "get")
	_ = checker.Check(ctx, "default", "results", "get")
	if tokenReviewCount.Load() != 1 {
		t.Fatalf("expected unauthenticated token cached (1 TokenReview), got %d", tokenReviewCount.Load())
	}
}

func TestCachingChecker_EvaluationErrorNotCached(t *testing.T) {
	var sarCount atomic.Int32

	k8s := fake.NewSimpleClientset()
	k8s.PrependReactor("create", "tokenreviews", func(action test.Action) (bool, runtime.Object, error) {
		tr := action.(test.CreateActionImpl).Object.(*authnv1.TokenReview)
		tr.Status = authnv1.TokenReviewStatus{
			Authenticated: true,
			User:          authnv1.UserInfo{Username: "test-user"},
		}
		return true, tr, nil
	})
	k8s.PrependReactor("create", "subjectaccessreviews", func(action test.Action) (bool, runtime.Object, error) {
		sarCount.Add(1)
		sar := action.(test.CreateActionImpl).Object.(*authzv1.SubjectAccessReview)
		sar.Status = authzv1.SubjectAccessReviewStatus{
			Allowed:         false,
			EvaluationError: "authorizer timeout",
		}
		return true, sar, nil
	})

	checker := auth.NewCachingChecker(auth.NewRBAC(k8s), 5*time.Minute, 5*time.Minute)
	t.Cleanup(checker.Stop)

	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("authorization", "Bearer test-token"))

	_ = checker.Check(ctx, "default", "results", "get")
	_ = checker.Check(ctx, "default", "results", "get")
	if sarCount.Load() != 2 {
		t.Fatalf("expected EvaluationError not cached (2 SAR calls), got %d", sarCount.Load())
	}
}

func TestCachingChecker_AllowedDespiteEvaluationError(t *testing.T) {
	var sarCount atomic.Int32

	k8s := fake.NewSimpleClientset()
	k8s.PrependReactor("create", "tokenreviews", func(action test.Action) (bool, runtime.Object, error) {
		tr := action.(test.CreateActionImpl).Object.(*authnv1.TokenReview)
		tr.Status = authnv1.TokenReviewStatus{
			Authenticated: true,
			User:          authnv1.UserInfo{Username: "test-user"},
		}
		return true, tr, nil
	})
	k8s.PrependReactor("create", "subjectaccessreviews", func(action test.Action) (bool, runtime.Object, error) {
		sarCount.Add(1)
		sar := action.(test.CreateActionImpl).Object.(*authzv1.SubjectAccessReview)
		sar.Status = authzv1.SubjectAccessReviewStatus{
			Allowed:         true,
			EvaluationError: "partial authorizer failure",
		}
		return true, sar, nil
	})

	checker := auth.NewCachingChecker(auth.NewRBAC(k8s), 5*time.Minute, 5*time.Minute)
	t.Cleanup(checker.Stop)

	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("authorization", "Bearer test-token"))

	if err := checker.Check(ctx, "default", "results", "get"); err != nil {
		t.Fatalf("Allowed+EvaluationError should succeed, got %v", err)
	}
	if err := checker.Check(ctx, "default", "results", "get"); err != nil {
		t.Fatalf("cached Allowed+EvaluationError should succeed, got %v", err)
	}
	if sarCount.Load() != 1 {
		t.Fatalf("expected Allowed decision cached (1 SAR), got %d", sarCount.Load())
	}
}

func TestCachingChecker_TokenReviewStatusErrorNotCached(t *testing.T) {
	var tokenReviewCount atomic.Int32

	k8s := fake.NewSimpleClientset()
	k8s.PrependReactor("create", "tokenreviews", func(action test.Action) (bool, runtime.Object, error) {
		n := tokenReviewCount.Add(1)
		tr := action.(test.CreateActionImpl).Object.(*authnv1.TokenReview)
		if n == 1 {
			tr.Status = authnv1.TokenReviewStatus{
				Authenticated: false,
				Error:         "tokenreviews.authentication.k8s.io could not be checked",
			}
			return true, tr, nil
		}
		tr.Status = authnv1.TokenReviewStatus{
			Authenticated: true,
			User:          authnv1.UserInfo{Username: "test-user"},
		}
		return true, tr, nil
	})
	k8s.PrependReactor("create", "subjectaccessreviews", func(action test.Action) (bool, runtime.Object, error) {
		sar := action.(test.CreateActionImpl).Object.(*authzv1.SubjectAccessReview)
		sar.Status = authzv1.SubjectAccessReviewStatus{Allowed: true}
		return true, sar, nil
	})

	checker := auth.NewCachingChecker(auth.NewRBAC(k8s), 5*time.Minute, 5*time.Minute)
	t.Cleanup(checker.Stop)

	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("authorization", "Bearer flaky-token"))

	if err := checker.Check(ctx, "default", "results", "get"); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("first Check with Status.Error = %v, want Unauthenticated", err)
	}
	if err := checker.Check(ctx, "default", "results", "get"); err != nil {
		t.Fatalf("second Check after recovered TokenReview failed: %v", err)
	}
	if tokenReviewCount.Load() != 2 {
		t.Fatalf("expected Status.Error not cached (2 TokenReviews), got %d", tokenReviewCount.Load())
	}
}

func TestCachingChecker_AuthzCacheExpiry(t *testing.T) {
	var sarCount atomic.Int32

	k8s := fake.NewSimpleClientset()
	k8s.PrependReactor("create", "tokenreviews", func(action test.Action) (bool, runtime.Object, error) {
		tr := action.(test.CreateActionImpl).Object.(*authnv1.TokenReview)
		tr.Status = authnv1.TokenReviewStatus{
			Authenticated: true,
			User:          authnv1.UserInfo{Username: "test-user"},
		}
		return true, tr, nil
	})
	k8s.PrependReactor("create", "subjectaccessreviews", func(action test.Action) (bool, runtime.Object, error) {
		sarCount.Add(1)
		sar := action.(test.CreateActionImpl).Object.(*authzv1.SubjectAccessReview)
		sar.Status = authzv1.SubjectAccessReviewStatus{Allowed: true}
		return true, sar, nil
	})

	checker := auth.NewCachingChecker(auth.NewRBAC(k8s), 5*time.Minute, 100*time.Millisecond)
	t.Cleanup(checker.Stop)

	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("authorization", "Bearer test-token"))

	if err := checker.Check(ctx, "default", "results", "get"); err != nil {
		t.Fatalf("first Check failed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := checker.Check(ctx, "default", "results", "get"); err != nil {
		t.Fatalf("Check after authz expiry failed: %v", err)
	}
	if sarCount.Load() != 2 {
		t.Fatalf("expected 2 SAR calls after authz TTL expiry, got %d", sarCount.Load())
	}
}

func TestCachingChecker_ExtraDistinguishesAuthzKeys(t *testing.T) {
	var sarCount atomic.Int32

	k8s := fake.NewSimpleClientset()
	k8s.PrependReactor("create", "tokenreviews", func(action test.Action) (bool, runtime.Object, error) {
		tr := action.(test.CreateActionImpl).Object.(*authnv1.TokenReview)
		extra := map[string]authnv1.ExtraValue{}
		switch tr.Spec.Token {
		case "token-authorized-extra":
			extra["scope"] = authnv1.ExtraValue{"authorized"}
		case "token-unauthorized-extra":
			extra["scope"] = authnv1.ExtraValue{"unauthorized"}
		}
		tr.Status = authnv1.TokenReviewStatus{
			Authenticated: true,
			User: authnv1.UserInfo{
				Username: "same-user",
				Groups:   []string{"same-group"},
				Extra:    extra,
			},
		}
		return true, tr, nil
	})
	k8s.PrependReactor("create", "subjectaccessreviews", func(action test.Action) (bool, runtime.Object, error) {
		sarCount.Add(1)
		sar := action.(test.CreateActionImpl).Object.(*authzv1.SubjectAccessReview)
		allowed := slices.Contains(sar.Spec.Extra["scope"], "authorized")
		sar.Status = authzv1.SubjectAccessReviewStatus{Allowed: allowed}
		return true, sar, nil
	})

	checker := auth.NewCachingChecker(auth.NewRBAC(k8s), 5*time.Minute, 5*time.Minute)
	t.Cleanup(checker.Stop)

	ctxAllowed := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("authorization", "Bearer token-authorized-extra"))
	ctxDenied := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("authorization", "Bearer token-unauthorized-extra"))

	if err := checker.Check(ctxAllowed, "default", "results", "get"); err != nil {
		t.Fatalf("authorized-extra Check failed: %v", err)
	}
	if err := checker.Check(ctxDenied, "default", "results", "get"); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("unauthorized-extra Check = %v, want Unauthenticated", err)
	}
	if sarCount.Load() != 2 {
		t.Fatalf("expected distinct authz cache keys (2 SAR calls), got %d", sarCount.Load())
	}
}

func TestCachingChecker_CacheExpiry(t *testing.T) {
	var tokenReviewCount atomic.Int32

	k8s := fake.NewSimpleClientset()
	k8s.PrependReactor("create", "tokenreviews", func(action test.Action) (bool, runtime.Object, error) {
		tokenReviewCount.Add(1)
		tr := action.(test.CreateActionImpl).Object.(*authnv1.TokenReview)
		tr.Status = authnv1.TokenReviewStatus{
			Authenticated: true,
			User:          authnv1.UserInfo{Username: "test-user"},
		}
		return true, tr, nil
	})
	k8s.PrependReactor("create", "subjectaccessreviews", func(action test.Action) (bool, runtime.Object, error) {
		sar := action.(test.CreateActionImpl).Object.(*authzv1.SubjectAccessReview)
		sar.Status = authzv1.SubjectAccessReviewStatus{Allowed: true}
		return true, sar, nil
	})

	checker := auth.NewCachingChecker(auth.NewRBAC(k8s), 100*time.Millisecond, 100*time.Millisecond)
	t.Cleanup(checker.Stop)

	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("authorization", "Bearer test-token"))

	if err := checker.Check(ctx, "default", "results", "get"); err != nil {
		t.Fatalf("first Check failed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := checker.Check(ctx, "default", "results", "get"); err != nil {
		t.Fatalf("Check after expiry failed: %v", err)
	}
	if tokenReviewCount.Load() != 2 {
		t.Fatalf("expected 2 TokenReview calls after expiry, got %d", tokenReviewCount.Load())
	}
}

func TestCachingChecker_RawTokenNotStoredInCacheKeys(t *testing.T) {
	rawToken := "super-secret-token-value"

	k8s := fake.NewSimpleClientset()
	k8s.PrependReactor("create", "tokenreviews", func(action test.Action) (bool, runtime.Object, error) {
		tr := action.(test.CreateActionImpl).Object.(*authnv1.TokenReview)
		tr.Status = authnv1.TokenReviewStatus{
			Authenticated: true,
			User:          authnv1.UserInfo{Username: "test-user"},
		}
		return true, tr, nil
	})
	k8s.PrependReactor("create", "subjectaccessreviews", func(action test.Action) (bool, runtime.Object, error) {
		sar := action.(test.CreateActionImpl).Object.(*authzv1.SubjectAccessReview)
		sar.Status = authzv1.SubjectAccessReviewStatus{Allowed: true}
		return true, sar, nil
	})

	checker := auth.NewCachingChecker(auth.NewRBAC(k8s), 5*time.Minute, 5*time.Minute)
	t.Cleanup(checker.Stop)

	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("authorization", "Bearer "+rawToken))
	if err := checker.Check(ctx, "default", "results", "get"); err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	for _, key := range checker.AuthnCacheKeys() {
		if key == rawToken || strings.Contains(key, rawToken) {
			t.Fatalf("authn cache key %q contains raw token", key)
		}
	}
	for _, key := range checker.AuthzCacheKeys() {
		if key == rawToken || strings.Contains(key, rawToken) {
			t.Fatalf("authz cache key %q contains raw token", key)
		}
	}
}

func TestRBAC_CacheDisabledHitsKubernetesEveryTime(t *testing.T) {
	var tokenReviewCount atomic.Int32
	var sarCount atomic.Int32

	k8s := fake.NewSimpleClientset()
	k8s.PrependReactor("create", "tokenreviews", func(action test.Action) (bool, runtime.Object, error) {
		tokenReviewCount.Add(1)
		tr := action.(test.CreateActionImpl).Object.(*authnv1.TokenReview)
		tr.Status = authnv1.TokenReviewStatus{
			Authenticated: true,
			User:          authnv1.UserInfo{Username: "test-user"},
		}
		return true, tr, nil
	})
	k8s.PrependReactor("create", "subjectaccessreviews", func(action test.Action) (bool, runtime.Object, error) {
		sarCount.Add(1)
		sar := action.(test.CreateActionImpl).Object.(*authzv1.SubjectAccessReview)
		sar.Status = authzv1.SubjectAccessReviewStatus{Allowed: true}
		return true, sar, nil
	})

	checker := auth.NewCheckerFromConfig(auth.NewRBAC(k8s), auth.CacheConfig{Enabled: false})
	if len(checker.AuthnCacheKeys()) != 0 || len(checker.AuthzCacheKeys()) != 0 {
		t.Fatal("expected no caches when AUTH_CACHE_ENABLED=false")
	}
	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("authorization", "Bearer test-token"))

	if err := checker.Check(ctx, "default", "results", "get"); err != nil {
		t.Fatalf("first Check failed: %v", err)
	}
	if err := checker.Check(ctx, "default", "results", "get"); err != nil {
		t.Fatalf("second Check failed: %v", err)
	}
	if tokenReviewCount.Load() != 2 || sarCount.Load() != 2 {
		t.Fatalf("expected 2/2 K8s calls without cache, got %d/%d", tokenReviewCount.Load(), sarCount.Load())
	}
}

func TestNewCheckerFromConfig_Enabled(t *testing.T) {
	var tokenReviewCount atomic.Int32

	k8s := fake.NewSimpleClientset()
	k8s.PrependReactor("create", "tokenreviews", func(action test.Action) (bool, runtime.Object, error) {
		tokenReviewCount.Add(1)
		tr := action.(test.CreateActionImpl).Object.(*authnv1.TokenReview)
		tr.Status = authnv1.TokenReviewStatus{
			Authenticated: true,
			User:          authnv1.UserInfo{Username: "test-user"},
		}
		return true, tr, nil
	})
	k8s.PrependReactor("create", "subjectaccessreviews", func(action test.Action) (bool, runtime.Object, error) {
		sar := action.(test.CreateActionImpl).Object.(*authzv1.SubjectAccessReview)
		sar.Status = authzv1.SubjectAccessReviewStatus{Allowed: true}
		return true, sar, nil
	})

	checker := auth.NewCheckerFromConfig(auth.NewRBAC(k8s), auth.CacheConfig{
		Enabled:  true,
		AuthnTTL: 5 * time.Minute,
		AuthzTTL: 5 * time.Minute,
	})
	t.Cleanup(checker.Stop)

	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("authorization", "Bearer test-token"))
	_ = checker.Check(ctx, "default", "results", "get")
	_ = checker.Check(ctx, "default", "results", "get")
	if tokenReviewCount.Load() != 1 {
		t.Fatalf("expected cache enabled (1 TokenReview), got %d", tokenReviewCount.Load())
	}
}

func TestCachingChecker_APIVersionDistinguishesAuthzKeys(t *testing.T) {
	var sarCount atomic.Int32
	versions := make([]string, 0, 2)

	k8s := fake.NewSimpleClientset()
	k8s.PrependReactor("create", "tokenreviews", func(action test.Action) (bool, runtime.Object, error) {
		tr := action.(test.CreateActionImpl).Object.(*authnv1.TokenReview)
		tr.Status = authnv1.TokenReviewStatus{
			Authenticated: true,
			User:          authnv1.UserInfo{Username: "test-user"},
		}
		return true, tr, nil
	})
	k8s.PrependReactor("create", "subjectaccessreviews", func(action test.Action) (bool, runtime.Object, error) {
		sarCount.Add(1)
		sar := action.(test.CreateActionImpl).Object.(*authzv1.SubjectAccessReview)
		versions = append(versions, sar.Spec.ResourceAttributes.Version)
		sar.Status = authzv1.SubjectAccessReviewStatus{Allowed: true}
		return true, sar, nil
	})

	checker := auth.NewCachingChecker(auth.NewRBAC(k8s), 5*time.Minute, 5*time.Minute)
	t.Cleanup(checker.Stop)

	base := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("authorization", "Bearer test-token"))
	for _, version := range []string{"", "v1"} {
		ctx := auth.WithResourceAttributes(base, auth.ResourceAttributes{
			Group: "results.tekton.dev", Version: version,
		})
		if err := checker.Check(ctx, "default", "results", "get"); err != nil {
			t.Fatalf("Check version %q failed: %v", version, err)
		}
	}
	if sarCount.Load() != 2 {
		t.Fatalf("expected 2 SAR calls for distinct API versions, got %d", sarCount.Load())
	}
	if len(versions) != 2 || versions[0] != "" || versions[1] != "v1" {
		t.Fatalf("SAR versions=%v, want [\"\", \"v1\"]", versions)
	}
}

func TestCachingChecker_Impersonation(t *testing.T) {
	var sarCount atomic.Int32

	k8s := fake.NewSimpleClientset()
	k8s.PrependReactor("create", "tokenreviews", func(action test.Action) (bool, runtime.Object, error) {
		tr := action.(test.CreateActionImpl).Object.(*authnv1.TokenReview)
		tr.Status = authnv1.TokenReviewStatus{
			Authenticated: true,
			User:          authnv1.UserInfo{Username: "admin"},
		}
		return true, tr, nil
	})
	k8s.PrependReactor("create", "subjectaccessreviews", func(action test.Action) (bool, runtime.Object, error) {
		sarCount.Add(1)
		sar := action.(test.CreateActionImpl).Object.(*authzv1.SubjectAccessReview)
		switch {
		case sar.Spec.ResourceAttributes != nil && sar.Spec.ResourceAttributes.Resource == "users" &&
			sar.Spec.ResourceAttributes.Verb == "impersonate":
			sar.Status = authzv1.SubjectAccessReviewStatus{Allowed: true}
		case sar.Spec.User == "authorized":
			sar.Status = authzv1.SubjectAccessReviewStatus{Allowed: true}
		case sar.Spec.User == "unauthorized":
			sar.Status = authzv1.SubjectAccessReviewStatus{Allowed: false}
		default:
			sar.Status = authzv1.SubjectAccessReviewStatus{Allowed: false}
		}
		return true, sar, nil
	})

	checker := auth.NewCachingChecker(auth.NewRBAC(k8s, auth.WithImpersonation(true)), 5*time.Minute, 5*time.Minute)
	t.Cleanup(checker.Stop)

	ctx := metadata.NewIncomingContext(context.Background(), metadata.New(map[string]string{
		"authorization":    "Bearer admin-token",
		"Impersonate-User": "authorized",
	}))
	if err := checker.Check(ctx, "default", "results", "get"); err != nil {
		t.Fatalf("impersonate authorized failed: %v", err)
	}
	afterFirst := sarCount.Load()
	if afterFirst < 2 {
		t.Fatalf("expected impersonation+resource SARs on first Check, got %d", afterFirst)
	}

	if err := checker.Check(ctx, "default", "results", "get"); err != nil {
		t.Fatalf("impersonate authorized cache hit failed: %v", err)
	}
	if sarCount.Load() != afterFirst {
		t.Fatalf("expected impersonation SARs cached (still %d), got %d", afterFirst, sarCount.Load())
	}

	ctxDenied := metadata.NewIncomingContext(context.Background(), metadata.New(map[string]string{
		"authorization":    "Bearer admin-token",
		"Impersonate-User": "unauthorized",
	}))
	if err := checker.Check(ctxDenied, "default", "results", "get"); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("impersonate unauthorized = %v, want Unauthenticated", err)
	}
}
