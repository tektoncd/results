package config

import (
	"errors"
	"fmt"

	"k8s.io/client-go/rest"
)

// getHostURLs retrieves the external access URLs for Tekton Results API.
// It automatically detects the platform (OpenShift vs Kubernetes) and uses the appropriate method.
//
// Parameters:
//   - c: A pointer to a rest.Config struct containing the Kubernetes REST configuration.
//   - resultsNamespace: The namespace where Tekton Results is installed. If empty, uses platform defaults.
//
// Returns:
//   - A slice of strings containing the external access URLs.
//   - An error if any step in the process fails.
func getHostURLs(c *rest.Config, resultsNamespace string) ([]string, error) {
	if c == nil {
		return nil, errors.New("nil REST config provided")
	}

	platform := DetectPlatform(c)

	// Use provided namespace or default based on platform
	namespace := resultsNamespace
	if namespace == "" {
		namespace = GetDefaultResultsNamespace(platform)
	}

	switch platform {
	case PlatformOpenShift:
		return getOpenShiftRoutes(c, namespace)
	case PlatformKubernetes:
		return getKubernetesIngress(c, namespace)
	default:
		return nil, errors.New("unable to detect platform type")
	}
}

// getOpenShiftRoutes retrieves OpenShift routes for Tekton Results API
func getOpenShiftRoutes(c *rest.Config, namespace string) ([]string, error) {
	routes, err := getRoutes(c, namespace)
	if err != nil {
		return nil, err
	}

	if len(routes) == 0 {
		return nil, fmt.Errorf("no Tekton Results routes found in namespace %s.\n\nEither:\n  1. Create a route for the tekton-results-api-service\n  2. Use manual configuration: tkn-results config set --host=<url> --token=<token> --api-path=<path>\n  3. Check if Results is installed in a different namespace using --results-namespace", namespace)
	}

	return constructRouteURLs(routes), nil
}

// getKubernetesIngress retrieves Kubernetes ingresses for Tekton Results API
func getKubernetesIngress(c *rest.Config, namespace string) ([]string, error) {
	ingresses, err := getIngresses(c, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to detect ingresses: %w.\n\nTry manual configuration:\n  tkn-results config set --host=<url> --token=<token> --api-path=<path>", err)
	}

	if len(ingresses) == 0 {
		return nil, fmt.Errorf("no Tekton Results ingresses found in namespace %s.\n\nEither:\n  1. Create an ingress for the tekton-results-api-service\n  2. Use manual configuration: tkn-results config set --host=<url> --token=<token> --api-path=<path>\n  3. Check if Results is installed in a different namespace using --results-namespace", namespace)
	}

	return constructIngressURLs(ingresses), nil
}
