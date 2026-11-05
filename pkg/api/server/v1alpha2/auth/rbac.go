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
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
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
	"knative.dev/pkg/logging"
)

var errTokenNotAuthenticated = errors.New("token not authenticated")

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
	clock              Clock

	// Optional caches enabled by WithCaching. Nil means caching is off.
	authnCache       *Cache
	authzCache       *Cache
	negativeCacheTTL time.Duration
	cacheOptions     *cacheOptions
}

// authnResult holds the authenticated user identity from a TokenReview.
type authnResult struct {
	Username string
	UID      string
	Groups   []string
	Extra    map[string]authnv1.ExtraValue
}

// Option is configuration option for RBAC checker.
type Option func(*RBAC)

// NewRBAC returns new instance of the Kubernetes RBAC based auth checker.
func NewRBAC(client kubernetes.Interface, options ...Option) *RBAC {
	rbac := &RBAC{
		authn: client.AuthenticationV1(),
		authz: client.AuthorizationV1(),
		clock: realClock{},
	}
	for _, option := range options {
		option(rbac)
	}
	rbac.initializeCaches()
	return rbac
}

// AuthnCacheKeys returns authn cache keys for tests. Nil when caching is disabled.
func (r *RBAC) AuthnCacheKeys() []string {
	if r.authnCache == nil {
		return nil
	}
	return r.authnCache.Keys()
}

// AuthzCacheKeys returns authz cache keys for tests. Nil when caching is disabled.
func (r *RBAC) AuthzCacheKeys() []string {
	if r.authzCache == nil {
		return nil
	}
	return r.authzCache.Keys()
}

// authenticate validates a bearer token via the Kubernetes TokenReview API
// and returns the authenticated user's identity. When caching is enabled,
// successful and definitive unauthenticated results are cached. Transport
// failures and TokenReview Status.Error responses are not cached so transient
// auth-backend outages are retried.
func (r *RBAC) authenticate(ctx context.Context, token string) (*authnResult, error) {
	tokenHash := hashString(token)
	if r.authnCache != nil {
		if cached, ok := r.authnCache.Get(tokenHash); ok {
			if cached == nil {
				return nil, errTokenNotAuthenticated
			}
			return cached.(*authnResult), nil
		}
	}

	tr, err := r.authn.TokenReviews().Create(ctx, &authnv1.TokenReview{
		Spec: authnv1.TokenReviewSpec{
			Token: token,
		},
	}, metav1.CreateOptions{})
	if err != nil {
		// Do not cache transport / API-server errors.
		return nil, fmt.Errorf("token review failed: %w", err)
	}
	if !tr.Status.Authenticated {
		// Status.Error means the token could not be checked (transient).
		// Do not negative-cache that outcome.
		if tr.Status.Error != "" {
			return nil, fmt.Errorf("token review error: %s", tr.Status.Error)
		}
		if r.authnCache != nil {
			r.authnCache.SetWithTTL(tokenHash, nil, r.negativeCacheTTL)
		}
		return nil, errTokenNotAuthenticated
	}

	result := &authnResult{
		Username: tr.Status.User.Username,
		UID:      tr.Status.User.UID,
		Groups:   tr.Status.User.Groups,
		Extra:    tr.Status.User.Extra,
	}
	if r.authnCache != nil {
		// Positive authn entries must not outlive the JWT exp claim when
		// present. TokenReview remains the authority; exp only caps TTL.
		r.authnCache.SetWithTTL(tokenHash, result, authnCacheTTL(token, r.authnCache.TTL(), r.clock.Now()))
	}
	return result, nil
}

// authnCacheTTL returns the TTL for a successful TokenReview cache entry:
// the earlier of the configured cache TTL and the JWT exp claim (when
// parseable). Non-JWT tokens keep the configured TTL.
func authnCacheTTL(token string, configured time.Duration, now time.Time) time.Duration {
	exp, ok := jwtExpiry(token)
	if !ok {
		return configured
	}
	untilExp := exp.Sub(now)
	if untilExp <= 0 {
		return 0
	}
	if untilExp < configured {
		return untilExp
	}
	return configured
}

// jwtExpiry extracts exp from an unverified JWT payload. Returns false when
// the token is not a JWT or has no exp claim. Signature is not validated —
// Kubernetes TokenReview remains the authentication authority.
func jwtExpiry(token string) (time.Time, bool) {
	parsed, _, err := new(jwt.Parser).ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		return time.Time{}, false
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return time.Time{}, false
	}
	exp, ok := claims["exp"].(float64)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(int64(exp), 0), true
}

// authorize checks whether the given user is allowed to perform the specified
// action via the Kubernetes SubjectAccessReview API. When caching is enabled,
// definitive allow/deny results are cached. An Allowed decision is honored even
// when EvaluationError is also set. Pure EvaluationError responses (no allow)
// are not cached so transient authorizer failures are retried.
func (r *RBAC) authorize(ctx context.Context, user, uid string, groups []string, extra map[string]authzv1.ExtraValue, namespace, resource, verb string) (bool, error) {
	attrs := ResourceAttributesFrom(ctx)
	key := buildAuthzKey(user, uid, groups, extra, namespace, attrs.Group, attrs.Version, resource, attrs.Name, attrs.Subresource, verb)
	if r.authzCache != nil {
		if cached, ok := r.authzCache.Get(key); ok {
			return cached.(bool), nil
		}
	}

	sar, err := r.authz.SubjectAccessReviews().Create(ctx, &authzv1.SubjectAccessReview{
		Spec: authzv1.SubjectAccessReviewSpec{
			User:   user,
			UID:    uid,
			Groups: groups,
			Extra:  extra,
			ResourceAttributes: &authzv1.ResourceAttributes{
				Namespace:   namespace,
				Group:       attrs.Group,
				Version:     attrs.Version,
				Resource:    resource,
				Name:        attrs.Name,
				Subresource: attrs.Subresource,
				Verb:        verb,
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return false, err
	}
	if sar.Status.Allowed {
		if r.authzCache != nil {
			r.authzCache.Set(key, true)
		}
		return true, nil
	}
	if sar.Status.EvaluationError != "" {
		// Do not cache authorizer timeouts / partial failures.
		return false, fmt.Errorf("subject access review evaluation error: %s", sar.Status.EvaluationError)
	}

	if r.authzCache != nil {
		r.authzCache.SetWithTTL(key, false, r.negativeCacheTTL)
	}
	return false, nil
}

// checkImpersonation verifies the requester may impersonate each requested
// identity. When caching is enabled, each impersonation SAR is cached under a
// structured key derived from the requester and ResourceAttributes.
func (r *RBAC) checkImpersonation(ctx context.Context, impersonator *impersonation.Impersonation, requester string) error {
	attrs := impersonator.ResourceAttributes()
	if len(attrs) == 0 {
		return impersonation.ErrNoImpersonationData
	}
	for _, ra := range attrs {
		key := buildImpersonationAuthzKey(requester, ra)
		if r.authzCache != nil {
			if cached, ok := r.authzCache.Get(key); ok {
				if !cached.(bool) {
					return fmt.Errorf("forbidden: '%s' doesn't have permission to impersonate '%s'", requester, ra.Resource)
				}
				continue
			}
		}

		sar, err := r.authz.SubjectAccessReviews().Create(ctx, &authzv1.SubjectAccessReview{
			Spec: authzv1.SubjectAccessReviewSpec{
				User: requester,
				ResourceAttributes: &authzv1.ResourceAttributes{
					Group:       ra.Group,
					Name:        ra.Name,
					Namespace:   ra.Namespace,
					Resource:    ra.Resource,
					Subresource: ra.Subresource,
					Verb:        ra.Verb,
				},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		if sar.Status.Allowed {
			if r.authzCache != nil {
				r.authzCache.Set(key, true)
			}
			continue
		}
		if sar.Status.EvaluationError != "" {
			return fmt.Errorf("subject access review evaluation error: %s", sar.Status.EvaluationError)
		}
		if r.authzCache != nil {
			r.authzCache.SetWithTTL(key, false, r.negativeCacheTTL)
		}
		return fmt.Errorf("forbidden: '%s' doesn't have permission to impersonate '%s'", requester, ra.Resource)
	}
	return nil
}

// Check determines if resource can be accessed with impersonation metadata stored in the context.
func (r *RBAC) Check(ctx context.Context, namespace, resource, verb string) error {
	logger := logging.FromContext(ctx)
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
			logger.Warnw("Invalid impersonation data", "error", err)
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
			logger.Debug("Unknown authorization header format")
			continue
		}
		t := s[1]

		authn, err := r.authenticate(ctx, t)
		if err != nil {
			// Backend / transport failures are operationally important and
			// should surface at Warn. Ordinary invalid-token results stay Debug.
			if errors.Is(err, errTokenNotAuthenticated) {
				logger.Debugw("Authentication failed", "error", err)
			} else {
				logger.Warnw("TokenReview backend failure", "error", err)
			}
			continue
		}

		user := authn.Username
		uid := authn.UID
		groups := authn.Groups
		extra := convertExtra[authnv1.ExtraValue](authn.Extra)

		// Check whether the authenticated user has permission to impersonate.
		// Impersonation SARs share the authz cache when caching is enabled.
		if impersonator != nil {
			if err := r.checkImpersonation(ctx, impersonator, user); err != nil {
				logger.Warnw("Impersonation authorization failed", "error", err, "user", user)
				retMsg = fmt.Sprintf("%s: %s", retMsg, err.Error())
				return status.Error(codes.Unauthenticated, retMsg)
			}
			// Change user data to impersonated user
			userInfo := impersonator.GetUserInfo()
			user = userInfo.GetName()
			uid = userInfo.GetUID()
			groups = userInfo.GetGroups()
			extra = convertExtra[[]string](userInfo.GetExtra())
		}

		allowed, err := r.authorize(ctx, user, uid, groups, extra, namespace, resource, verb)
		if err != nil {
			retMsg = fmt.Sprintf("%s user %q in groups %q employing %q against %q: %s", retMsg, user, groups, verb, resource, err.Error())
			logger.Warnw("Authorization check failed",
				"error", err,
				"user", user,
				"groups", groups,
				"verb", verb,
				"resource", resource,
			)
			continue
		}
		if allowed {
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
