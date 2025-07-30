package config

import (
	"context"
	"fmt"
	"strings"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// getIngresses retrieves Kubernetes ingresses for Tekton Results API
func getIngresses(c *rest.Config, namespace string) ([]*networkingv1.Ingress, error) {
	if c == nil {
		return nil, fmt.Errorf("nil REST config provided")
	}

	client, err := kubernetes.NewForConfig(c)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return getIngressesWithClient(client, namespace)
}

// getIngressesWithClient retrieves Kubernetes ingresses using the provided client
func getIngressesWithClient(client kubernetes.Interface, namespace string) ([]*networkingv1.Ingress, error) {
	ctx := context.Background()

	// Check if namespace exists
	_, err := client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		if strings.Contains(err.Error(), "forbidden") || strings.Contains(err.Error(), "permission") {
			return nil, fmt.Errorf("insufficient permissions to access namespace %s. Ask admin to setup RBAC permissions for ingresses access", namespace)
		}
		return nil, fmt.Errorf("namespace %s not found or no permission to access: %w", namespace, err)
	}

	// List ingresses in the namespace
	ingressList, err := client.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if strings.Contains(err.Error(), "forbidden") || strings.Contains(err.Error(), "permission") {
			return nil, fmt.Errorf("insufficient permissions to list ingresses in namespace %s. Ask admin to setup RBAC permissions for ingresses access", namespace)
		}
		return nil, fmt.Errorf("failed to list ingresses in namespace %s: %w", namespace, err)
	}

	var resultIngresses []*networkingv1.Ingress
	for _, ingress := range ingressList.Items {
		if isTektonResultsIngress(ingress) {
			// Create a copy to avoid issues with range variable
			ingressCopy := ingress
			resultIngresses = append(resultIngresses, &ingressCopy)
		}
	}

	return resultIngresses, nil
}

// isTektonResultsIngress checks if an ingress is for Tekton Results API
func isTektonResultsIngress(ingress networkingv1.Ingress) bool {
	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP != nil {
			for _, path := range rule.HTTP.Paths {
				if path.Backend.Service != nil {
					serviceName := path.Backend.Service.Name
					if serviceName == "tekton-results-api-service" {
						return true
					}
				}
			}
		}
	}
	return false
}

// constructIngressURLs builds URLs from ingress configuration
func constructIngressURLs(ingresses []*networkingv1.Ingress) []string {
	var urls []string
	for _, ingress := range ingresses {
		for _, rule := range ingress.Spec.Rules {
			if rule.Host != "" {
				scheme := "http"
				// Check if this host has TLS configuration
				for _, tls := range ingress.Spec.TLS {
					for _, host := range tls.Hosts {
						if host == rule.Host {
							scheme = "https"
							break
						}
					}
					if scheme == "https" {
						break
					}
				}
				urls = append(urls, fmt.Sprintf("%s://%s", scheme, rule.Host))
			}
		}
	}
	return urls
}
