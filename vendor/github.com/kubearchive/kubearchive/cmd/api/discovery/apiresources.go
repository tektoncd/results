// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package discovery

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	"github.com/kubearchive/kubearchive/pkg/abort"
	"github.com/kubearchive/kubearchive/pkg/cache"
)

// Arbitrary time for cache, probably enough to not overload the Kubernetes API
var cacheExpirationTime = 10 * time.Minute

// GetAPIResource set apiResource attribute in the context based on the path parameters group version and resourceType
//
// This function relied on the DiscoveryClient's ServerResourcesForGroupVersion function. However it does not accept a context
// so we had to implement the same call to be able to pass the context so telemetry traces are tied together
// @rh-hemartin opened a ticket to allow for a context on that function, see https://github.com/kubernetes/client-go/issues/1370
// The client-go discovery package offers cache implementations however our cache implementation is simpler.
func GetAPIResource(client rest.Interface, cache *cache.Cache) gin.HandlerFunc {
	return func(c *gin.Context) {
		resourceName := c.Param("resourceType")
		kind := cache.Get(resourceName)
		if kind != nil {
			c.Set("apiResourceKind", kind)
			return
		}

		discoveryURL := getDiscoveryURL(c.Param("group"), c.Param("version"))
		result := client.Get().AbsPath(discoveryURL).Do(c.Request.Context())

		if result.Error() != nil {
			status := 0
			result.StatusCode(&status)
			abort.Abort(c, fmt.Errorf("unable to retrieve information from '%s', error: %w", discoveryURL, result.Error()), status)
			return
		}

		resources := &metav1.APIResourceList{}
		err := result.Into(resources)
		if err != nil {
			abort.Abort(c, fmt.Errorf("unable to deserialize result from '%s', error: %w", discoveryURL, err), http.StatusInternalServerError)
			return
		}

		for _, resource := range resources.APIResources {
			if resource.Name == resourceName {
				cache.Set(resourceName, resource.Kind, cacheExpirationTime)
				c.Set("apiResourceKind", resource.Kind)
				return
			}
		}
		abort.Abort(c,
			fmt.Errorf("unable to find the API resource %s in the Kubernetes cluster", resourceName),
			http.StatusNotFound)
	}
}

func GetAPIResourceKind(context *gin.Context) (string, error) {
	kind := context.GetString("apiResourceKind")
	if kind == "" {
		return "", errors.New("API resource not found")
	}

	return kind, nil
}

func getDiscoveryURL(group, version string) string {
	//  Core resource case: /api as root, just version
	url := fmt.Sprintf("/api/%s", version)
	if group != "" {
		// Non Core resource case: /apis as root, group and version used
		url = fmt.Sprintf("/apis/%s/%s", group, version)
	}

	return url
}
