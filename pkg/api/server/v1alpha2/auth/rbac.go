// Copyright 2021 The Tekton Authors
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
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth/impersonation"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	authnv1 "k8s.io/api/authentication/v1"
	authzv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	authnclient "k8s.io/client-go/kubernetes/typed/authentication/v1"
	authzclient "k8s.io/client-go/kubernetes/typed/authorization/v1"
)

// RBAC is a Kubernetes RBAC based auth checker. This uses the Kubernetes
// TokenReview and SubjectAccessReview APIs to defer auth decisions to the
// cluster.
// Users should pass in `token` metadata through the gRPC context.
// This checks RBAC permissions in the `results.tekton.dev` group, and assumes
// checks are done at the namespace
type RBAC struct {
	allowImpersonation bool
	authn              authnclient.AuthenticationV1Interface
	authz              authzclient.AuthorizationV1Interface
}

// Option is configuration option for RBAC checker.
type Option func(*RBAC)

// NewRBAC returns new instance of the Kubernetes RBAC based auth checker.
func NewRBAC(client kubernetes.Interface, options ...Option) *RBAC {
	rbac := &RBAC{
		authn: client.AuthenticationV1(),
		authz: client.AuthorizationV1(),
	}
	for _, option := range options {
		option(rbac)
	}
	return rbac
}

// Check determines if resource can be accessed with impersonation metadata stored in the context.
func (r *RBAC) Check(ctx context.Context, namespace, resource, verb string) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "unable to get context metadata")
	}

	// Parse Impersonation header if the feature is enabled
	var impersonator *impersonation.Impersonation
	var err error
	if r.allowImpersonation {
		impersonator, err = impersonation.NewImpersonation(md)
		// Ignore ErrorNoImpersonationData errors. This means that the request does not have any
		// impersonation headers and should be processed normally.
		if err != nil && !errors.Is(err, impersonation.ErrNoImpersonationData) {
			log.Println(err)
			return status.Error(codes.Unauthenticated, "invalid impersonation data")
		}
	}

	v := md.Get("authorization")
	if len(v) == 0 {
		return status.Error(codes.Unauthenticated, "unable to find token")
	}

	if verb == PermissionList && namespace == "-" {
		// In list operations `-` means that the caller wants to list
		// resources across all parents. Thus, let's assume all
		// namespaces here.
		namespace = corev1.NamespaceAll
	}

	retMsg := "permission denied"
	for _, raw := range v {
		// We expect tokens to be in the form "Bearer <token>". Parse the token out.
		s := strings.SplitN(raw, " ", 2)
		if len(s) < 2 {
			log.Println("unknown auth token format")
			continue
		}
		t := s[1]

		// Authenticate the token by sending it to the API Server for review.
		tr, err := r.authn.TokenReviews().Create(ctx, &authnv1.TokenReview{
			Spec: authnv1.TokenReviewSpec{
				Token: t,
			},
		}, metav1.CreateOptions{})
		if err != nil {
			log.Println(err)
			continue
		}
		if !tr.Status.Authenticated {
			continue
		}

		user := tr.Status.User.Username
		UID := tr.Status.User.UID
		groups := tr.Status.User.Groups
		extra := convertExtra[authnv1.ExtraValue](tr.Status.User.Extra)

		// Check whether the authenticated user has permission to impersonate
		if impersonator != nil {
			if err := impersonator.Check(ctx, r.authz, user); err != nil {
				log.Println(err)
				retMsg = fmt.Sprintf("%s: %s", retMsg, err.Error())
				return status.Error(codes.Unauthenticated, retMsg)
			}
			// Change user data to impersonated user
			userInfo := impersonator.GetUserInfo()
			user = userInfo.GetName()
			UID = userInfo.GetUID()
			groups = userInfo.GetGroups()
			extra = convertExtra[[]string](userInfo.GetExtra())
		}

		// Authorize the request by checking the RBAC permissions for the resource.
		sar, err := r.authz.SubjectAccessReviews().Create(ctx, &authzv1.SubjectAccessReview{
			Spec: authzv1.SubjectAccessReviewSpec{
				User:   user,
				UID:    UID,
				Groups: groups,
				Extra:  extra,
				ResourceAttributes: &authzv1.ResourceAttributes{
					Namespace: namespace,
					Group:     "results.tekton.dev",
					Resource:  resource,
					Verb:      verb,
				},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			retMsg = fmt.Sprintf("%s user %q in groups %q employing %q against %q: %s", retMsg, user, groups, verb, resource, err.Error())
			log.Println(err)
			continue
		}
		if sar.Status.Allowed {
			return nil
		}
	}
	// Return Unauthenticated - we don't know if we failed because of invalid
	// token or unauthorized user, so this is safer to not leak any state.
	return status.Error(codes.Unauthenticated, retMsg)
}

// convertExtra converts the map[string][]string or authnv1.ExtraValue to map[string]ExtraValue for Subject Access Review.
func convertExtra[T []string | authnv1.ExtraValue](extra map[string]T) map[string]authzv1.ExtraValue {
	var newExtra = make(map[string]authzv1.ExtraValue)
	for key, value := range extra {
		newExtra[key] = authzv1.ExtraValue(value)
	}
	return newExtra
}

// WithImpersonation is an option function to enable Impersonation
func WithImpersonation(enabled bool) Option {
	return func(r *RBAC) {
		r.allowImpersonation = enabled
	}
}
