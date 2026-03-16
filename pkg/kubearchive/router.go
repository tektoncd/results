// Copyright 2020 The Tekton Authors
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

package kubearchive

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/cmd/api/auth"
	"github.com/kubearchive/kubearchive/cmd/api/discovery"
	"github.com/kubearchive/kubearchive/cmd/api/pagination"
	"github.com/kubearchive/kubearchive/cmd/api/routers"
	"github.com/kubearchive/kubearchive/pkg/cache"
	"github.com/tektoncd/results/pkg/kubearchive/adapter"
	"k8s.io/client-go/kubernetes"
)

// NewRouter creates a new Gin router configured with Kubearchive's kubectl-compatible API
// It accepts any API group for querying archived Tekton resources
func NewRouter(
	k8sClient kubernetes.Interface,
	dbAdapter *adapter.TektonResultsAdapter,
	cache *cache.Cache,
	cacheConfig *routers.CacheExpirations,
) http.Handler {
	router := gin.New()
	router.Use(gin.Recovery())

	// Create route groups matching Kubearchive's structure
	apisGroup := router.Group("/apis")

	// Apply Kubearchive middleware chain:
	// 1. Authentication (TokenReview)
	// 2. Impersonation (optional)
	// 3. API Resource Discovery
	// 4. Authorization (SubjectAccessReview)
	// 5. Pagination validation

	apisGroup.Use(auth.Authentication(
		k8sClient.AuthenticationV1().TokenReviews(),
		cache,
		cacheConfig.Authorized,
		cacheConfig.Unauthorized,
	))

	apisGroup.Use(auth.Impersonation(
		k8sClient.AuthorizationV1().SubjectAccessReviews(),
		cache,
		cacheConfig.Authorized,
		cacheConfig.Unauthorized,
	))

	// Discovery middleware is required because it sets the
	// "apiResourceKind" parameter in the Gin context, which is passed
	// to controller.GetResources
	apisGroup.Use(discovery.GetAPIResource(
		k8sClient.Discovery().RESTClient(),
		cache,
	))

	apisGroup.Use(auth.RBACAuthorization(
		k8sClient.AuthorizationV1().SubjectAccessReviews(),
		cache,
		cacheConfig.Authorized,
		cacheConfig.Unauthorized,
	))

	apisGroup.Use(pagination.Middleware())

	// Setup Kubearchive controller with our adapter
	controller := routers.Controller{
		Database:           dbAdapter,
		CacheConfiguration: *cacheConfig,
	}

	// Mount routes with :group parameter (matching Kubearchive's pattern)
	// Full path is /apis/:group/:version/:resourceType
	// Any API group can be requested - no restrictions for proof of concept
	apisGroup.GET("/:group/:version/:resourceType", controller.GetResources)
	apisGroup.GET("/:group/:version/namespaces/:namespace/:resourceType", controller.GetResources)
	apisGroup.GET("/:group/:version/namespaces/:namespace/:resourceType/:name", controller.GetResources)

	// Return the Gin engine as an http.Handler
	return router
}
