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

// Package k8s provides Kubernetes-style HTTP authentication/authorization
// middleware for Results API routes. It reuses the shared auth.Checker so
// TokenReview/SubjectAccessReview caching configured for the gRPC path also
// applies here.
package k8s

import (
	"context"
	"net/http"
	"strings"

	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth/impersonation"
	"google.golang.org/grpc/metadata"
	authnv1 "k8s.io/api/authentication/v1"
)

// ResourceAttrs describes the Kubernetes resource a request targets.
// Every field is included in SubjectAccessReview attributes and authz cache
// keys so decisions for different objects or subresources do not collide.
type ResourceAttrs struct {
	Namespace   string
	Group       string
	Version     string
	Resource    string
	Name        string
	Subresource string
	Verb        string
}

type attrsKey struct{}

// WithResourceAttrs attaches authorization attributes for Middleware.
func WithResourceAttrs(ctx context.Context, attrs ResourceAttrs) context.Context {
	return context.WithValue(ctx, attrsKey{}, attrs)
}

// ResourceAttrsFrom returns attributes previously attached with WithResourceAttrs.
func ResourceAttrsFrom(ctx context.Context) (ResourceAttrs, bool) {
	attrs, ok := ctx.Value(attrsKey{}).(ResourceAttrs)
	return attrs, ok
}

// Auth authorizes Kubernetes-style HTTP requests using a shared auth.Checker.
type Auth struct {
	checker auth.Checker
}

// NewAuth returns HTTP auth middleware backed by checker. Pass the same
// checker instance used for the gRPC Results API so caches are shared.
func NewAuth(checker auth.Checker) *Auth {
	return &Auth{checker: checker}
}

// Middleware authorizes requests that already have ResourceAttrs in context.
// On success it forwards to next; on failure it responds 401 Unauthorized.
func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attrs, ok := ResourceAttrsFrom(r.Context())
		if !ok {
			http.Error(w, "missing resource authorization attributes", http.StatusInternalServerError)
			return
		}
		if err := a.Authorize(r, attrs); err != nil {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Handler returns a handler that authorizes with fixed attrs then calls next.
// Useful when registering Kubernetes-style HTTP routes that share the Results
// auth checker and its caches.
func (a *Auth) Handler(attrs ResourceAttrs, next http.Handler) http.Handler {
	authorized := a.Middleware(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(WithResourceAttrs(r.Context(), attrs))
		authorized.ServeHTTP(w, r)
	})
}

// Authorize runs the shared Checker against the HTTP request. Bearer and
// Impersonation headers are mapped into gRPC metadata so RBAC.Check works
// unchanged; resource details are attached for SAR attributes and cache keys.
func (a *Auth) Authorize(r *http.Request, attrs ResourceAttrs) error {
	ctx := contextFromRequest(r.Context(), r.Header)
	ctx = auth.WithResourceAttributes(ctx, auth.ResourceAttributes{
		Group:       attrs.Group,
		Version:     attrs.Version,
		Name:        attrs.Name,
		Subresource: attrs.Subresource,
	})
	return a.checker.Check(ctx, attrs.Namespace, attrs.Resource, attrs.Verb)
}

// contextFromRequest copies Authorization and Impersonation headers into
// incoming gRPC metadata expected by auth.RBAC.
func contextFromRequest(ctx context.Context, header http.Header) context.Context {
	md := metadata.MD{}
	if v := header.Get("Authorization"); v != "" {
		md.Set("authorization", v)
	}
	for _, key := range []string{
		authnv1.ImpersonateUserHeader,
		authnv1.ImpersonateGroupHeader,
		authnv1.ImpersonateUIDHeader,
	} {
		if vals := header.Values(key); len(vals) > 0 {
			md.Set(key, vals...)
		}
	}
	for name, values := range header {
		if !strings.HasPrefix(strings.ToLower(name), strings.ToLower(authnv1.ImpersonateUserExtraHeaderPrefix)) {
			continue
		}
		matched, _ := impersonation.HeaderMatcher(name)
		md.Set(matched, values...)
	}
	return metadata.NewIncomingContext(ctx, md)
}
