package impersonation

import (
	"context"
	"errors"
	"fmt"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc/metadata"
	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	authorizationclient "k8s.io/client-go/kubernetes/typed/authorization/v1"

	"net/url"
	"strings"

	"k8s.io/apiserver/pkg/authentication/serviceaccount"
)

// Impersonation represents component to add Kubernetes impersonation header processing,
// make impersonation access check and RBAC for Tekton results resources with impersonated user data
type Impersonation struct {
	resourceAttributes []authorizationv1.ResourceAttributes
	userInfo           *user.DefaultInfo
}

var (
	// ErrNoImpersonationData is an error message for no impersonation data case
	ErrNoImpersonationData = errors.New("no impersonation data found")
	// ErrImpersonateUserRequired is an error message about required impersonate user to impersonate another info.
	ErrImpersonateUserRequired = errors.New("impersonate user is required to impersonate groups, UID, extra")
)

// NewImpersonation returns an impersonation request if any impersonation data is found, returns error otherwise.
func NewImpersonation(md metadata.MD) (*Impersonation, error) {
	i := &Impersonation{}
	if err := i.parseMetadata(md); err != nil {
		return nil, err
	}
	return i, nil
}

// parseMetadata parses the gRPC metadata and returns a lst of object references that represents the Impersonation data.
func (i *Impersonation) parseMetadata(md metadata.MD) error {
	i.userInfo = &user.DefaultInfo{}
	users := md.Get(authenticationv1.ImpersonateUserHeader)
	hasUser := len(users) > 0
	if hasUser {
		if namespace, name, err := serviceaccount.SplitUsername(users[0]); err == nil {
			i.userInfo.Name = serviceaccount.MakeUsername(namespace, name)
			i.resourceAttributes = append(i.resourceAttributes, authorizationv1.ResourceAttributes{
				Name:      name,
				Namespace: namespace,
				Resource:  "serviceaccounts",
				Verb:      "impersonate",
			})

			// If groups aren't specified for a service account, we know the groups because it's a fixed mapping.
			if len(md.Get(authenticationv1.ImpersonateGroupHeader)) == 0 {
				i.userInfo.Groups = serviceaccount.MakeGroupNames(namespace)
			}
		} else {
			i.userInfo.Name = users[0]
			i.resourceAttributes = append(i.resourceAttributes, authorizationv1.ResourceAttributes{
				Name:     users[0],
				Resource: "users",
				Verb:     "impersonate",
			})
		}
	}

	groups := md.Get(authenticationv1.ImpersonateGroupHeader)
	hasGroups := len(groups) > 0
	if hasGroups {
		for _, group := range groups {
			i.userInfo.Groups = append(i.userInfo.Groups, group)
			i.resourceAttributes = append(i.resourceAttributes, authorizationv1.ResourceAttributes{
				Name:     group,
				Resource: "groups",
				Verb:     "impersonate",
			})
		}
	}

	UIDs := md.Get(authenticationv1.ImpersonateUIDHeader)
	hasUID := len(UIDs) > 0
	if hasUID {
		i.userInfo.UID = UIDs[0]
		i.resourceAttributes = append(i.resourceAttributes, authorizationv1.ResourceAttributes{
			Group:    authenticationv1.SchemeGroupVersion.Group,
			Name:     UIDs[0],
			Resource: "uids",
			Verb:     "impersonate",
		})
	}

	hasExtra := false
	for name, values := range md {
		if !strings.HasPrefix(name, strings.ToLower(authenticationv1.ImpersonateUserExtraHeaderPrefix)) {
			continue
		}

		hasExtra = true
		extraKey := unescapeExtraKey(strings.ToLower(name[len(authenticationv1.ImpersonateUserExtraHeaderPrefix):]))
		i.userInfo.Extra = make(map[string][]string)
		// Each extra value is a separate resource to check.
		for _, value := range values {
			i.userInfo.Extra[extraKey] = append(i.userInfo.Extra[extraKey], value)
			i.resourceAttributes = append(i.resourceAttributes, authorizationv1.ResourceAttributes{
				Group:       authenticationv1.SchemeGroupVersion.Group,
				Name:        value,
				Resource:    "userextras",
				Subresource: extraKey,
				Verb:        "impersonate",
			})
		}
	}

	// When impersonating a non-anonymous user, include the 'system:authenticated' group in the impersonated user info:
	// - if no groups were specified
	// - if a group has been specified other than 'system:authenticated'
	// If 'system:unauthenticated' group has been specified we should not include the 'system:authenticated' group.
	if hasUser {
		if i.userInfo.Name != user.Anonymous {
			addAuthenticated := true
			for _, group := range i.userInfo.Groups {
				if group == user.AllAuthenticated || group == user.AllUnauthenticated {
					addAuthenticated = false
					break
				}
			}

			if addAuthenticated {
				i.userInfo.Groups = append(i.userInfo.Groups, user.AllAuthenticated)
			}
		} else {
			addUnauthenticated := true
			for _, group := range i.userInfo.Groups {
				if group == user.AllUnauthenticated {
					addUnauthenticated = false
					break
				}
			}

			if addUnauthenticated {
				i.userInfo.Groups = append(i.userInfo.Groups, user.AllUnauthenticated)
			}
		}
	} else {
		i.userInfo = nil

		// Impersonate-User header is mandatory.
		// https://kubernetes.io/docs/reference/access-authn-authz/authentication/#user-impersonation
		if hasGroups || hasExtra || hasUID {
			return ErrImpersonateUserRequired
		}
		return ErrNoImpersonationData
	}

	return nil
}

func unescapeExtraKey(encodedKey string) string {
	key, err := url.PathUnescape(encodedKey) // Decode %-encoded bytes.
	if err != nil {
		return encodedKey // Always record extra strings, even if malformed/unencoded.
	}
	return key
}

// Check checks if the requester has permission to impersonate every resource.
func (i *Impersonation) Check(ctx context.Context, authorizer authorizationclient.AuthorizationV1Interface, requester string) error {
	if i.resourceAttributes == nil {
		return ErrNoImpersonationData
	}
	for _, resourceAttribute := range i.resourceAttributes {
		sar, err := authorizer.SubjectAccessReviews().Create(ctx, &authorizationv1.SubjectAccessReview{
			Spec: authorizationv1.SubjectAccessReviewSpec{
				User: requester,
				ResourceAttributes: &authorizationv1.ResourceAttributes{
					Group:       resourceAttribute.Group,
					Name:        resourceAttribute.Name,
					Namespace:   resourceAttribute.Namespace,
					Resource:    resourceAttribute.Resource,
					Subresource: resourceAttribute.Subresource,
					Verb:        resourceAttribute.Verb,
				},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		if !sar.Status.Allowed {
			return fmt.Errorf("forbidden: '%s' doesn't have permission to impersonate '%s'", requester, resourceAttribute.Resource)
		}
	}

	return nil
}

// GetUserInfo returns the impersonated user information.
func (i *Impersonation) GetUserInfo() *user.DefaultInfo {
	return i.userInfo
}

// HeaderMatcher matches the impersonation header for adding to GRPC metadata.
func HeaderMatcher(key string) (string, bool) {
	if strings.HasPrefix(key, authenticationv1.ImpersonateUserExtraHeaderPrefix) {
		return key, true
	}

	switch key {
	case authenticationv1.ImpersonateUserHeader:
		return key, true
	case authenticationv1.ImpersonateGroupHeader:
		return key, true
	case authenticationv1.ImpersonateUIDHeader:
		return key, true
	default:
		return runtime.DefaultHeaderMatcher(key)
	}
}
