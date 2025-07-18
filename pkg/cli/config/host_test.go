package config

import (
	"testing"

	v1 "github.com/openshift/api/route/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

// TestGetRoutes tests the getRoutes function
func TestGetRoutes(t *testing.T) {
	tests := []struct {
		name       string
		config     *rest.Config
		wantErr    bool
		wantRoutes bool
	}{
		{
			name: "invalid config - kubernetes environment",
			config: &rest.Config{
				Host: "invalid-host",
			},
			wantErr:    true,
			wantRoutes: false,
		},
		{
			name:       "nil config",
			config:     nil,
			wantErr:    true,
			wantRoutes: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routes, err := getRoutes(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("getRoutes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(routes) > 0 != tt.wantRoutes {
				t.Errorf("getRoutes() routes = %v, wantRoutes %v", len(routes) > 0, tt.wantRoutes)
			}
		})
	}
}

// TestGetRoutesWithNilConfig tests getRoutes with nil config
func TestGetRoutesWithNilConfig(t *testing.T) {
	routes, err := getRoutes(nil)
	if err == nil {
		t.Error("Expected error with nil config")
	}
	if routes != nil {
		t.Error("Expected nil routes with nil config")
	}
}

func TestHostMethod(t *testing.T) {
	// Test that Host method works without parameters
	config := &config{
		RESTConfig: &rest.Config{},
	}

	// Test that the method can be called without panicking
	result := config.Host()
	if result == nil {
		t.Log("Host method called successfully")
	}
}

func TestIsTektonResultsRoute(t *testing.T) {
	tests := []struct {
		name     string
		route    v1.Route
		expected bool
	}{
		{
			name: "route pointing to tekton-results-api-service service",
			route: v1.Route{
				Spec: v1.RouteSpec{
					To: v1.RouteTargetReference{
						Name: "tekton-results-api-service",
					},
				},
			},
			expected: true,
		},
		{
			name: "route pointing to tekton-results-api service",
			route: v1.Route{
				Spec: v1.RouteSpec{
					To: v1.RouteTargetReference{
						Name: "tekton-results-api",
					},
				},
			},
			expected: true,
		},
		{
			name: "route pointing to tekton-results service",
			route: v1.Route{
				Spec: v1.RouteSpec{
					To: v1.RouteTargetReference{
						Name: "tekton-results",
					},
				},
			},
			expected: true,
		},
		{
			name: "route pointing to other service",
			route: v1.Route{
				Spec: v1.RouteSpec{
					To: v1.RouteTargetReference{
						Name: "other-service",
					},
				},
			},
			expected: false,
		},
		{
			name: "route with other name but not pointing to tekton-results service",
			route: v1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name: "other-route",
				},
				Spec: v1.RouteSpec{
					To: v1.RouteTargetReference{
						Name: "other-service",
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTektonResultsRoute(tt.route)
			if result != tt.expected {
				t.Errorf("isTektonResultsRoute() = %v, want %v", result, tt.expected)
			}
		})
	}
}
