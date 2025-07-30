package config

import (
	"context"
	"testing"

	v1 "github.com/openshift/api/route/v1"
	routeapplyv1 "github.com/openshift/client-go/route/applyconfigurations/route/v1"
	routev1 "github.com/openshift/client-go/route/clientset/versioned/typed/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

// mockRouteInterface implements routev1.RouteInterface for testing
type mockRouteInterface struct {
	routes []v1.Route
}

func (m *mockRouteInterface) List(_ context.Context, _ metav1.ListOptions) (*v1.RouteList, error) {
	return &v1.RouteList{Items: m.routes}, nil
}

// Implement other required methods as no-ops for testing
func (m *mockRouteInterface) Get(_ context.Context, _ string, _ metav1.GetOptions) (*v1.Route, error) {
	return nil, nil
}
func (m *mockRouteInterface) Create(_ context.Context, _ *v1.Route, _ metav1.CreateOptions) (*v1.Route, error) {
	return nil, nil
}
func (m *mockRouteInterface) Update(_ context.Context, _ *v1.Route, _ metav1.UpdateOptions) (*v1.Route, error) {
	return nil, nil
}
func (m *mockRouteInterface) UpdateStatus(_ context.Context, _ *v1.Route, _ metav1.UpdateOptions) (*v1.Route, error) {
	return nil, nil
}
func (m *mockRouteInterface) Delete(_ context.Context, _ string, _ metav1.DeleteOptions) error {
	return nil
}
func (m *mockRouteInterface) DeleteCollection(_ context.Context, _ metav1.DeleteOptions, _ metav1.ListOptions) error {
	return nil
}
func (m *mockRouteInterface) Patch(_ context.Context, _ string, _ types.PatchType, _ []byte, _ metav1.PatchOptions, _ ...string) (result *v1.Route, err error) {
	return nil, nil
}
func (m *mockRouteInterface) Apply(_ context.Context, _ *routeapplyv1.RouteApplyConfiguration, _ metav1.ApplyOptions) (result *v1.Route, err error) {
	return nil, nil
}
func (m *mockRouteInterface) ApplyStatus(_ context.Context, _ *routeapplyv1.RouteApplyConfiguration, _ metav1.ApplyOptions) (result *v1.Route, err error) {
	return nil, nil
}
func (m *mockRouteInterface) Watch(_ context.Context, _ metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

// mockRouteV1Interface implements routev1.RouteV1Interface for testing
type mockRouteV1Interface struct {
	routesByNamespace map[string][]v1.Route
}

func (m *mockRouteV1Interface) Routes(namespace string) routev1.RouteInterface {
	routes := m.routesByNamespace[namespace]
	return &mockRouteInterface{routes: routes}
}

func (m *mockRouteV1Interface) RESTClient() rest.Interface {
	return nil
}

// TestGetRoutesWithClients tests the actual getRoutesWithClients function
func TestGetRoutesWithClients(t *testing.T) {
	tests := []struct {
		name          string
		namespace     string
		routes        []v1.Route
		expectedCount int
		expectedError bool
		description   string
	}{
		{
			name:          "no tekton results routes",
			namespace:     "openshift-pipelines",
			routes:        []v1.Route{},
			expectedCount: 0,
			expectedError: true, // Function returns error when no tekton routes found
			description:   "should return error when no tekton results routes found",
		},
		{
			name:      "tekton results route found",
			namespace: "openshift-pipelines",
			routes: []v1.Route{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tekton-results-api",
						Namespace: "openshift-pipelines",
					},
					Spec: v1.RouteSpec{
						Host: "tekton-results-api-openshift-pipelines.apps.cluster.local",
						To: v1.RouteTargetReference{
							Kind: "Service",
							Name: "tekton-results-api-service",
						},
					},
				},
			},
			expectedCount: 1,
			expectedError: false,
			description:   "should find tekton results route",
		},
		{
			name:      "mixed routes with filtering",
			namespace: "openshift-pipelines",
			routes: []v1.Route{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tekton-results-api",
						Namespace: "openshift-pipelines",
					},
					Spec: v1.RouteSpec{
						Host: "tekton-results-api-openshift-pipelines.apps.cluster.local",
						To: v1.RouteTargetReference{
							Kind: "Service",
							Name: "tekton-results-api-service",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-service",
						Namespace: "openshift-pipelines",
					},
					Spec: v1.RouteSpec{
						Host: "other-service-openshift-pipelines.apps.cluster.local",
						To: v1.RouteTargetReference{
							Kind: "Service",
							Name: "other-service",
						},
					},
				},
			},
			expectedCount: 1,
			expectedError: false,
			description:   "should filter and find only tekton results routes",
		},
		{
			name:      "multiple tekton results routes",
			namespace: "openshift-pipelines",
			routes: []v1.Route{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tekton-results-api-1",
						Namespace: "openshift-pipelines",
					},
					Spec: v1.RouteSpec{
						Host: "tekton-results-1-openshift-pipelines.apps.cluster.local",
						To: v1.RouteTargetReference{
							Kind: "Service",
							Name: "tekton-results-api-service",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tekton-results-api-2",
						Namespace: "openshift-pipelines",
					},
					Spec: v1.RouteSpec{
						Host: "tekton-results-2-openshift-pipelines.apps.cluster.local",
						To: v1.RouteTargetReference{
							Kind: "Service",
							Name: "tekton-results-api-service",
						},
						TLS: &v1.TLSConfig{
							Termination: v1.TLSTerminationEdge,
						},
					},
				},
			},
			expectedCount: 2,
			expectedError: false,
			description:   "should find multiple tekton results routes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake kubernetes clientset with namespace
			k8sObjects := []runtime.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: tt.namespace,
					},
				},
			}
			fakeK8sClient := fake.NewClientset(k8sObjects...)

			// Create mock route client
			routesByNamespace := map[string][]v1.Route{
				tt.namespace: tt.routes,
			}
			mockRouteClient := &mockRouteV1Interface{
				routesByNamespace: routesByNamespace,
			}

			// Test the actual getRoutesWithClients function
			routes, err := getRoutesWithClients(mockRouteClient, fakeK8sClient, tt.namespace)

			// Check error expectations
			if tt.expectedError {
				if err == nil {
					t.Errorf("%s: expected error but got none", tt.description)
				}
				return
			}

			if err != nil {
				t.Errorf("%s: unexpected error: %v", tt.description, err)
				return
			}

			// Check the count
			if len(routes) != tt.expectedCount {
				t.Errorf("%s: expected %d tekton results routes, got %d",
					tt.description, tt.expectedCount, len(routes))
			}

			// Verify each found route is indeed a tekton results route
			for _, route := range routes {
				if !isTektonResultsRoute(*route) {
					t.Errorf("%s: route %s/%s should be a tekton results route",
						tt.description, route.Namespace, route.Name)
				}
			}

			// Test URL construction integration
			if len(routes) > 0 {
				urls := constructRouteURLs(routes)
				if len(urls) == 0 {
					t.Errorf("%s: expected URLs to be constructed, got none", tt.description)
				}

				// Verify URLs are properly constructed
				for i, url := range urls {
					route := routes[i]
					expectedScheme := "http"
					if route.Spec.TLS != nil {
						expectedScheme = "https"
					}
					expectedURL := expectedScheme + "://" + route.Spec.Host
					if url != expectedURL {
						t.Errorf("%s: expected URL %q, got %q",
							tt.description, expectedURL, url)
					}
				}
			}
		})
	}
}

// TestIsTektonResultsRoute tests the isTektonResultsRoute function
func TestIsTektonResultsRoute(t *testing.T) {
	tests := []struct {
		name        string
		route       v1.Route
		expected    bool
		description string
	}{
		{
			name: "route pointing to tekton-results-api-service",
			route: v1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tekton-results-api",
					Namespace: "openshift-pipelines",
				},
				Spec: v1.RouteSpec{
					To: v1.RouteTargetReference{
						Kind: "Service",
						Name: "tekton-results-api-service",
					},
					Host: "tekton-results-api-openshift-pipelines.apps.cluster.local",
				},
			},
			expected:    true,
			description: "should return true for route pointing to tekton-results-api-service",
		},
		{
			name: "route pointing to other service",
			route: v1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-route",
					Namespace: "openshift-pipelines",
				},
				Spec: v1.RouteSpec{
					To: v1.RouteTargetReference{
						Kind: "Service",
						Name: "other-service",
					},
					Host: "other-service-openshift-pipelines.apps.cluster.local",
				},
			},
			expected:    false,
			description: "should return false for route pointing to other service",
		},
		{
			name: "route with different target kind",
			route: v1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tekton-results",
					Namespace: "openshift-pipelines",
				},
				Spec: v1.RouteSpec{
					To: v1.RouteTargetReference{
						Kind: "DeploymentConfig",
						Name: "tekton-results-api-service",
					},
					Host: "tekton-results-openshift-pipelines.apps.cluster.local",
				},
			},
			expected:    true,
			description: "should return true for route pointing to tekton-results service regardless of target kind",
		},
		{
			name: "route with empty target name",
			route: v1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-target",
					Namespace: "openshift-pipelines",
				},
				Spec: v1.RouteSpec{
					To: v1.RouteTargetReference{
						Kind: "Service",
						Name: "",
					},
					Host: "empty-target-openshift-pipelines.apps.cluster.local",
				},
			},
			expected:    false,
			description: "should return false for route with empty target name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTektonResultsRoute(tt.route)
			if result != tt.expected {
				t.Errorf("%s: isTektonResultsRoute() = %v, want %v", tt.description, result, tt.expected)
			}
		})
	}
}

// TestConstructRouteURLs tests the constructRouteURLs function
func TestConstructRouteURLs(t *testing.T) {
	tests := []struct {
		name         string
		routes       []*v1.Route
		expectedURLs []string
		description  string
	}{
		{
			name:         "empty routes",
			routes:       []*v1.Route{},
			expectedURLs: []string{},
			description:  "should return empty URLs for empty routes",
		},
		{
			name:         "nil routes",
			routes:       nil,
			expectedURLs: []string{},
			description:  "should return empty URLs for nil routes",
		},
		{
			name: "route with HTTP only",
			routes: []*v1.Route{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tekton-results-api",
						Namespace: "openshift-pipelines",
					},
					Spec: v1.RouteSpec{
						Host: "tekton-results-api-openshift-pipelines.apps.cluster.local",
						To: v1.RouteTargetReference{
							Kind: "Service",
							Name: "tekton-results-api-service",
						},
					},
				},
			},
			expectedURLs: []string{"http://tekton-results-api-openshift-pipelines.apps.cluster.local"},
			description:  "should return HTTP URL for route without TLS",
		},
		{
			name: "route with HTTPS (TLS configured)",
			routes: []*v1.Route{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tekton-results-api-secure",
						Namespace: "openshift-pipelines",
					},
					Spec: v1.RouteSpec{
						Host: "tekton-results-api-openshift-pipelines.apps.cluster.local",
						To: v1.RouteTargetReference{
							Kind: "Service",
							Name: "tekton-results-api-service",
						},
						TLS: &v1.TLSConfig{
							Termination: v1.TLSTerminationEdge,
						},
					},
				},
			},
			expectedURLs: []string{"https://tekton-results-api-openshift-pipelines.apps.cluster.local"},
			description:  "should return HTTPS URL for route with TLS",
		},
		{
			name: "multiple routes",
			routes: []*v1.Route{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tekton-results-api-http",
						Namespace: "openshift-pipelines",
					},
					Spec: v1.RouteSpec{
						Host: "tekton-results-http-openshift-pipelines.apps.cluster.local",
						To: v1.RouteTargetReference{
							Kind: "Service",
							Name: "tekton-results-api-service",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tekton-results-api-https",
						Namespace: "openshift-pipelines",
					},
					Spec: v1.RouteSpec{
						Host: "tekton-results-https-openshift-pipelines.apps.cluster.local",
						To: v1.RouteTargetReference{
							Kind: "Service",
							Name: "tekton-results-api-service",
						},
						TLS: &v1.TLSConfig{
							Termination: v1.TLSTerminationEdge,
						},
					},
				},
			},
			expectedURLs: []string{
				"http://tekton-results-http-openshift-pipelines.apps.cluster.local",
				"https://tekton-results-https-openshift-pipelines.apps.cluster.local",
			},
			description: "should return URLs for multiple routes",
		},
		{
			name: "route with different TLS termination types",
			routes: []*v1.Route{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tekton-results-edge",
						Namespace: "openshift-pipelines",
					},
					Spec: v1.RouteSpec{
						Host: "tekton-results-edge-openshift-pipelines.apps.cluster.local",
						To: v1.RouteTargetReference{
							Kind: "Service",
							Name: "tekton-results-api-service",
						},
						TLS: &v1.TLSConfig{
							Termination: v1.TLSTerminationEdge,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tekton-results-passthrough",
						Namespace: "openshift-pipelines",
					},
					Spec: v1.RouteSpec{
						Host: "tekton-results-passthrough-openshift-pipelines.apps.cluster.local",
						To: v1.RouteTargetReference{
							Kind: "Service",
							Name: "tekton-results-api-service",
						},
						TLS: &v1.TLSConfig{
							Termination: v1.TLSTerminationPassthrough,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tekton-results-reencrypt",
						Namespace: "openshift-pipelines",
					},
					Spec: v1.RouteSpec{
						Host: "tekton-results-reencrypt-openshift-pipelines.apps.cluster.local",
						To: v1.RouteTargetReference{
							Kind: "Service",
							Name: "tekton-results-api-service",
						},
						TLS: &v1.TLSConfig{
							Termination: v1.TLSTerminationReencrypt,
						},
					},
				},
			},
			expectedURLs: []string{
				"https://tekton-results-edge-openshift-pipelines.apps.cluster.local",
				"https://tekton-results-passthrough-openshift-pipelines.apps.cluster.local",
				"https://tekton-results-reencrypt-openshift-pipelines.apps.cluster.local",
			},
			description: "should return HTTPS URLs for all TLS termination types",
		},
		{
			name: "route with empty host",
			routes: []*v1.Route{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tekton-results-no-host",
						Namespace: "openshift-pipelines",
					},
					Spec: v1.RouteSpec{
						Host: "",
						To: v1.RouteTargetReference{
							Kind: "Service",
							Name: "tekton-results-api-service",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tekton-results-with-host",
						Namespace: "openshift-pipelines",
					},
					Spec: v1.RouteSpec{
						Host: "tekton-results-openshift-pipelines.apps.cluster.local",
						To: v1.RouteTargetReference{
							Kind: "Service",
							Name: "tekton-results-api-service",
						},
					},
				},
			},
			expectedURLs: []string{"http://tekton-results-openshift-pipelines.apps.cluster.local"},
			description:  "should skip routes with empty hosts and return only valid URLs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := constructRouteURLs(tt.routes)

			// Compare lengths first
			if len(result) != len(tt.expectedURLs) {
				t.Errorf("%s: constructRouteURLs() returned %d URLs, want %d. Got: %v, Want: %v",
					tt.description, len(result), len(tt.expectedURLs), result, tt.expectedURLs)
				return
			}

			// Compare each URL
			for i, url := range result {
				if i >= len(tt.expectedURLs) || url != tt.expectedURLs[i] {
					t.Errorf("%s: constructRouteURLs()[%d] = %v, want %v", tt.description, i, url, tt.expectedURLs[i])
				}
			}
		})
	}
}
