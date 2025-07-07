package config

import (
	"context"
	"errors"

	v1 "github.com/openshift/api/route/v1"
	routev1 "github.com/openshift/client-go/route/clientset/versioned/typed/route/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// getRoutes retrieves the OpenShift routes associated with Tekton Results services.
//
// It uses the provided Kubernetes REST configuration to create clients for core and route resources.
// For OpenShift environments, it directly lists routes in the openshift-pipelines and tekton-results namespaces.
// For Kubernetes environments, it returns an error suggesting manual configuration.
//
// Parameters:
//   - c: A pointer to a rest.Config struct containing the Kubernetes REST configuration.
//
// Returns:
//   - A slice of pointers to v1.Route objects representing the matched routes.
//   - An error if any step in the process fails, including if no services or routes are found.
func getRoutes(c *rest.Config) ([]*v1.Route, error) {
	if c == nil {
		return nil, errors.New("nil REST config provided")
	}

	// Try to create OpenShift route client to detect if we're in OpenShift
	routeV1Client, err := routev1.NewForConfig(c)
	if err != nil {
		// If we can't create route client, we're likely in Kubernetes
		return nil, errors.New("automatic detection not available in Kubernetes environment. Please use manual configuration:\n  tkn-results config set --host=<url> --token=<token> --api-path=<path>")
	}

	// Create core client to check namespace existence
	coreV1Client, err := kubernetes.NewForConfig(c)
	if err != nil {
		return nil, err
	}

	// We're in OpenShift, try to list routes in both openshift-pipelines and tekton-results namespaces
	ctx := context.Background()
	namespaces := []string{"openshift-pipelines", "tekton-results"}
	var allRoutes []*v1.Route

	for _, namespace := range namespaces {
		// First check if namespace exists
		_, err := coreV1Client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if err != nil {
			// Continue to next namespace if this one doesn't exist
			continue
		}

		// List all routes in the namespace (no label selector)
		routeList, err := routeV1Client.Routes(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			// Continue to next namespace if this one fails
			continue
		}

		for _, route := range routeList.Items {
			// Check if this route is for Tekton Results API
			if isTektonResultsRoute(route) {
				allRoutes = append(allRoutes, &route)
			}
		}
	}

	if len(allRoutes) == 0 {
		return nil, errors.New("no Tekton Results routes found in openshift-pipelines or tekton-results namespaces, try manual configuration:\n  tkn-results config set --host=<url> --token=<token> --api-path=<path>")
	}

	return allRoutes, nil
}

// isTektonResultsRoute checks if a route is for Tekton Results API
func isTektonResultsRoute(route v1.Route) bool {
	// Check if route points to tekton-results service
	if route.Spec.To.Name == "tekton-results-api-service" || route.Spec.To.Name == "tekton-results-api" || route.Spec.To.Name == "tekton-results" {
		return true
	}

	return false
}
