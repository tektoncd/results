// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/pkg/abort"
	"github.com/kubearchive/kubearchive/pkg/cache"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apiAuthnv1 "k8s.io/api/authentication/v1"
	apiAuthzv1 "k8s.io/api/authorization/v1"
	"k8s.io/apiserver/pkg/authentication/serviceaccount"
	"k8s.io/apiserver/pkg/authentication/user"
	clientAuthzv1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
)

const impersonateFlag = "AUTH_IMPERSONATE"

var ErrNoImpersonationData = errors.New("no impersonation data found")

type impersonatedData struct {
	resourceAttributes []*apiAuthzv1.ResourceAttributes
	userInfo           user.Info
}

func newImpersonatedData(c *gin.Context) (*impersonatedData, error) {

	userInfo := &user.DefaultInfo{}
	resourceAtts := make([]*apiAuthzv1.ResourceAttributes, 0)
	var hasUser, hasGroups, hasUID, hasExtras bool

	userSarAtt, impersonatedUser, namespace := parseUser(c.Request.Header)
	if impersonatedUser != "" {
		hasUser = true
		userInfo.Name = impersonatedUser
		resourceAtts = append(resourceAtts, userSarAtt)
	}
	grpsSarAtts, groups := parseGroups(c.Request.Header)
	if len(groups) > 0 {
		hasGroups = true
		userInfo.Groups = groups
		resourceAtts = append(resourceAtts, grpsSarAtts...)
	} else if namespace != "" {
		// If no groups but it's a service account, the groups can be extracted from the namespace
		userInfo.Groups = serviceaccount.MakeGroupNames(namespace)
	}
	uidSarAtt, uid := parseUID(c.Request.Header)
	if uid != "" {
		hasUID = true
		userInfo.UID = uid
		resourceAtts = append(resourceAtts, uidSarAtt)
	}
	extrasSarAtts, extras := parseExtras(c.Request.Header)
	if len(extras) > 0 {
		hasExtras = true
		userInfo.Extra = extras
		resourceAtts = append(resourceAtts, extrasSarAtts...)
	}

	if !hasUser {
		if hasGroups || hasUID || hasExtras {
			return nil, fmt.Errorf("header %s required for impersonation", apiAuthnv1.ImpersonateUserHeader)
		}
		return nil, ErrNoImpersonationData
	}

	if userInfo.Name != user.Anonymous {
		// add 'system:authenticated' if it's not already provided and the user is not anonymous
		if !slices.Contains(groups, user.AllAuthenticated) {
			userInfo.Groups = append(userInfo.Groups, user.AllAuthenticated)
		}
	} else {
		// add 'system:unauthenticated' if it's not already provided and the user is anonymous
		if !slices.Contains(groups, user.AllUnauthenticated) {
			userInfo.Groups = append(userInfo.Groups, user.AllUnauthenticated)
		}
	}

	return &impersonatedData{resourceAttributes: resourceAtts, userInfo: userInfo}, nil
}

// parseUser returns the SAR ResourceAttribute for impersonation, the username and its namespace if it's a SA
// from HTTP impersonation headers
func parseUser(headers http.Header) (*apiAuthzv1.ResourceAttributes, string, string) {
	impersonatedUser := headers.Get(apiAuthnv1.ImpersonateUserHeader)
	if impersonatedUser == "" {
		return nil, "", ""
	}
	namespace, _, err := serviceaccount.SplitUsername(impersonatedUser)

	// service account
	if err == nil {
		return &apiAuthzv1.ResourceAttributes{
			Name:      impersonatedUser,
			Namespace: namespace,
			Resource:  "serviceaccounts",
			Verb:      "impersonate",
		}, impersonatedUser, namespace

	}

	// user
	return &apiAuthzv1.ResourceAttributes{
		Name:     impersonatedUser,
		Resource: "users",
		Verb:     "impersonate",
	}, impersonatedUser, ""

}

// parseGroups returns the SAR ResourceAttributes for impersonation and the groups from HTTP impersonation headers
func parseGroups(headers http.Header) ([]*apiAuthzv1.ResourceAttributes, []string) {
	groups := headers.Values(apiAuthnv1.ImpersonateGroupHeader)
	resourceAtts := make([]*apiAuthzv1.ResourceAttributes, 0)
	for _, group := range groups {
		resourceAtts = append(resourceAtts, &apiAuthzv1.ResourceAttributes{
			Name:     group,
			Resource: "groups",
			Verb:     "impersonate",
		})
	}
	return resourceAtts, groups
}

// parseUID returns the SAR ResourceAttribute for impersonation and the UID from HTTP impersonation headers
func parseUID(headers http.Header) (*apiAuthzv1.ResourceAttributes, string) {
	uid := headers.Get(apiAuthnv1.ImpersonateUIDHeader)
	if uid == "" {
		return nil, ""
	}
	return &apiAuthzv1.ResourceAttributes{
		Group:    apiAuthnv1.SchemeGroupVersion.Group,
		Name:     uid,
		Resource: "uids",
		Verb:     "impersonate",
	}, uid
}

// parseExtras returns the SAR ResourceAttributes for impersonation and the extras from HTTP impersonation headers
func parseExtras(headers http.Header) ([]*apiAuthzv1.ResourceAttributes, map[string][]string) {
	extras := make(map[string][]string)
	resourceAtts := make([]*apiAuthzv1.ResourceAttributes, 0)
	for headerKey, headerValues := range headers {
		if strings.HasPrefix(headerKey, apiAuthnv1.ImpersonateUserExtraHeaderPrefix) {
			encodedKey := strings.TrimPrefix(headerKey, apiAuthnv1.ImpersonateUserExtraHeaderPrefix)
			key, err := url.PathUnescape(encodedKey)
			if err != nil {
				key = encodedKey
			}
			for _, value := range headerValues {
				extras[key] = append(extras[key], value)
				resourceAtts = append(resourceAtts, &apiAuthzv1.ResourceAttributes{
					Group:       apiAuthnv1.SchemeGroupVersion.Group,
					Name:        value,
					Resource:    "userextras",
					Subresource: key,
					Verb:        "impersonate",
				})
			}
		}
	}
	return resourceAtts, extras
}

func Impersonation(
	sari clientAuthzv1.SubjectAccessReviewInterface,
	cache *cache.Cache,
	cacheExpirationAuthorized,
	cacheExpirationUnauthorized time.Duration) gin.HandlerFunc {

	if os.Getenv(impersonateFlag) != "true" {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	return func(c *gin.Context) {
		imp, imperErr := newImpersonatedData(c)
		if imperErr != nil && !errors.Is(imperErr, ErrNoImpersonationData) {
			abort.Abort(c, imperErr, http.StatusBadRequest)
			return
		}
		// No impersonated data so this middleware is skipped
		if imp == nil {
			c.Next()
			return
		}

		requester, ok := c.Get("user")
		if !ok {
			abort.Abort(c, errors.New("user not found in context"), http.StatusInternalServerError)
			return
		}
		requesterInfo, okCast := requester.(*user.DefaultInfo)
		if !okCast {
			abort.Abort(c, fmt.Errorf("unexpected user type in context: %T", requester), http.StatusInternalServerError)
			return
		}

		err := doSarRequests(
			c.Request.Context(),
			sari,
			requesterInfo,
			imp.resourceAttributes,
			cache,
			cacheExpirationAuthorized,
			cacheExpirationUnauthorized,
		)
		if err != nil {
			abort.Abort(c,
				status.Error(codes.Unauthenticated, "the user don't have permission to impersonate"),
				http.StatusUnauthorized,
			)
			return
		}
		// The context user is updated with the impersonated user info
		c.Set("user", imp.userInfo)
	}
}
