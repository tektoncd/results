// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"errors"
	"time"

	"github.com/kubearchive/kubearchive/pkg/cache"
	apiAuthzv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	clientAuthzv1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
)

var errUnauth = errors.New("unauthorized")

// doSarRequests checks if the current user can impersonate the user in impersonatedData through SAR or cache
func doSarRequests(
	ctx context.Context,
	sari clientAuthzv1.SubjectAccessReviewInterface,
	user user.Info,
	resourceAttributes []*apiAuthzv1.ResourceAttributes,
	cache *cache.Cache,
	cacheExpirationAuthorized,
	cacheExpirationUnauthorized time.Duration,
) error {
	if resourceAttributes == nil {
		return ErrNoImpersonationData
	}
	for _, resourceAttribute := range resourceAttributes {

		sarSpecExtra := make(map[string]apiAuthzv1.ExtraValue)
		for k, v := range user.GetExtra() {
			sarSpecExtra[k] = v // Explicit conversion
		}

		sarSpec := apiAuthzv1.SubjectAccessReviewSpec{
			User:               user.GetName(),
			UID:                user.GetUID(),
			Groups:             user.GetGroups(),
			ResourceAttributes: resourceAttribute,
			Extra:              sarSpecExtra,
		}

		allowed := cache.Get(sarSpec.String())
		if allowed != nil {
			if allowed != true {
				return errUnauth
			}
			continue
		}
		sar, err := sari.Create(ctx, &apiAuthzv1.SubjectAccessReview{Spec: sarSpec}, metav1.CreateOptions{})
		if err != nil {
			return err
		}

		cache.Set(sarSpec.String(), sar.Status.Allowed, cacheExpirationAuthorized)
		if !sar.Status.Allowed {
			cache.Set(sarSpec.String(), sar.Status.Allowed, cacheExpirationUnauthorized)
			return errUnauth
		}
	}
	return nil
}
