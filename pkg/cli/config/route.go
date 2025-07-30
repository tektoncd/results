package config

import (
	"context"
	"errors"
	"fmt"

	v1 "github.com/openshift/api/route/v1"
	routev1 "github.com/openshift/client-go/route/clientset/versioned/typed/route/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// getRoutes retrieves the OpenShift routes associated with Tekton Results services.
// This is the updated version that works with a specific namespace.
//
// Parameters:
//   - c: A pointer to a rest.Config struct containing the Kubernetes REST configuration.
//   - namespace: The specific namespace to search for routes.
//
// Returns:
//   - A slice of pointers to v1.Route objects representing the matched routes.
//   - An error if any step in the process fails, including if no services or routes are found.
func getRoutes(c *rest.Config, namespace string) ([]*v1.Route, error) {
	if c == nil {
		return nil, errors.New("nil REST config provided")
	}

	// Create OpenShift route client
	routeV1Client, err := routev1.NewForConfig(c)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenShift route client: %w", err)
	}

	// Create core client to check namespace existence
	coreV1Client, err := kubernetes.NewForConfig(c)
	if err != nil {
		return nil, err
	}

	return getRoutesWithClients(routeV1Client, coreV1Client, namespace)
}

// getRoutesWithClients retrieves OpenShift routes using the provided clients
func getRoutesWithClients(routeClient routev1.RouteV1Interface, coreClient kubernetes.Interface, namespace string) ([]*v1.Route, error) {
	ctx := context.Background()

	// Check if namespace exists
	_, err := coreClient.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("namespace %s not found or no permission to access: %w", namespace, err)
	}

	// List all routes in the namespace
	routeList, err := routeClient.Routes(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list routes in namespace %s: %w", namespace, err)
	}

	var allRoutes []*v1.Route
	for _, route := range routeList.Items {
		// Check if this route is for Tekton Results API
		if isTektonResultsRoute(route) {
			routeCopy := route
			allRoutes = append(allRoutes, &routeCopy)
		}
	}

	if len(allRoutes) == 0 {
		return nil, fmt.Errorf("no Tekton Results routes found in namespace %s, try manual configuration:\n  tkn-results config set --host=<url> --token=<token> --api-path=<path>", namespace)
	}

	return allRoutes, nil
}

// isTektonResultsRoute checks if a route is for Tekton Results API
func isTektonResultsRoute(route v1.Route) bool {
	// Check if route points to tekton-results service
	if route.Spec.To.Name == "tekton-results-api-service" {
		return true
	}

	return false
}

// constructRouteURLs builds URLs from route configuration
func constructRouteURLs(routes []*v1.Route) []string {
	var urls []string
	for _, route := range routes {
		// Skip routes with empty hosts
		if route.Spec.Host == "" {
			continue
		}

		host := "http://" + route.Spec.Host
		if route.Spec.TLS != nil {
			host = "https://" + route.Spec.Host
		}
		urls = append(urls, host)
	}
	return urls
}
