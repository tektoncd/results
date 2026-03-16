// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kubearchive/kubearchive/pkg/abort"
	"github.com/kubearchive/kubearchive/pkg/cache"
	"k8s.io/apiserver/pkg/authentication/user"

	"github.com/gin-gonic/gin"

	apiAuthzv1 "k8s.io/api/authorization/v1"
	clientAuthzv1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
)

func RBACAuthorization(
	sari clientAuthzv1.SubjectAccessReviewInterface,
	cache *cache.Cache,
	cacheExpirationAuthorized,
	cacheExpirationUnauthorized time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		usr, ok := c.Get("user")
		if !ok {
			abort.Abort(c, errors.New("user not found in context"), http.StatusInternalServerError)
			return
		}
		userInfo, ok := usr.(user.Info)
		if !ok {
			abort.Abort(c, fmt.Errorf("unexpected user type in context: %T", usr), http.StatusInternalServerError)
			return
		}

		verb := "list"
		if c.Param("name") != "" {
			verb = "get"
		}

		resourceAttributes := []*apiAuthzv1.ResourceAttributes{
			{
				Namespace: c.Param("namespace"),
				Group:     c.Param("group"),
				Version:   c.Param("version"),
				Resource:  c.Param("resourceType"),
				Name:      c.Param("name"),
				Verb:      verb,
			},
		}

		if strings.HasSuffix(c.Request.URL.Path, "/log") {
			resourceAttributes = append(resourceAttributes, &apiAuthzv1.ResourceAttributes{
				Namespace: c.Param("namespace"),
				Version:   "v1",
				Resource:  "pods/log",
				Verb:      "get",
			})
		}

		errSar := doSarRequests(
			c.Request.Context(),
			sari,
			userInfo,
			resourceAttributes,
			cache,
			cacheExpirationAuthorized,
			cacheExpirationUnauthorized,
		)

		if errSar != nil {
			if errors.Is(errSar, errUnauth) {
				abort.Abort(c, errSar, http.StatusUnauthorized)
				return
			}
			abort.Abort(c, errSar, http.StatusInternalServerError)
		}
	}
}
