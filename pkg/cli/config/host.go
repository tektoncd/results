package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"k8s.io/client-go/rest"
)

// getHostURL retrieves the external access URL for Tekton Results API.
// It automatically detects the platform and tries to connect to the standard tekton-results-api-service endpoint.
//
// Parameters:
//   - c: A pointer to a rest.Config struct containing the Kubernetes REST configuration.
//
// Returns:
//   - A string containing the external access URL.
//   - An error if any step in the process fails.
func getHostURL(c *rest.Config) (string, error) {
	if c == nil {
		return "", errors.New("nil REST config provided")
	}

	platform := DetectPlatform(c)

	switch platform {
	case PlatformOpenShift:
		return tryConnectToRoute(c)
	case PlatformKubernetes:
		return "", fmt.Errorf("kubernetes ingress not supported")
	default:
		return "", errors.New("unable to detect platform type")
	}
}

// tryConnectToRoute attempts to construct and test OpenShift route URLs to check the server's health
func tryConnectToRoute(c *rest.Config) (string, error) {
	clusterDomain, err := extractClusterDomain(c.Host)
	if err != nil {
		return "", fmt.Errorf("failed to extract cluster domain")
	}

	// OpenShift route patterns: tekton-results-api-service-{namespace}.apps.{cluster-domain}
	namespace := "openshift-pipelines"
	serviceName := "tekton-results-api-service"

	// Try HTTPS first (most common for OpenShift routes)
	httpsURL := fmt.Sprintf("https://%s-%s.apps.%s", serviceName, namespace, clusterDomain)
	if isURLReachable(httpsURL) {
		return httpsURL, nil
	}

	// Try HTTP as fallback
	httpURL := fmt.Sprintf("http://%s-%s.apps.%s", serviceName, namespace, clusterDomain)
	if isURLReachable(httpURL) {
		return httpURL, nil
	}
	return "", fmt.Errorf("no reachable route found")
}

// extractClusterDomain extracts the cluster domain from the Kubernetes API server URL
// Example: https://api.mycluster.example.com:6443 -> mycluster.example.com
func extractClusterDomain(apiServerURL string) (string, error) {
	if apiServerURL == "" {
		return "", errors.New("empty API server URL")
	}

	// Parse the URL
	u, err := url.Parse(apiServerURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL")
	}

	hostname := u.Hostname()
	if hostname == "" {
		return "", errors.New("failed to extract hostname")
	}

	// For OpenShift/K8s, API server is typically: api.{cluster-domain}
	// Extract {cluster-domain} part
	if strings.HasPrefix(hostname, "api.") {
		return strings.TrimPrefix(hostname, "api."), nil
	}

	// If it doesn't start with "api.", try to extract domain differently
	// Handle cases like: k8s-api-server.cluster.example.com -> cluster.example.com
	parts := strings.Split(hostname, ".")
	if len(parts) >= 2 {
		// Take the last two parts as domain (example.com)
		// or more if it looks like a full domain
		if len(parts) >= 3 {
			return strings.Join(parts[1:], "."), nil // Skip first part
		}
		return strings.Join(parts, "."), nil
	}

	return "", fmt.Errorf("unable to extract cluster domain")
}

// isURLReachable checks if a URL is reachable with a simple TCP connection test
func isURLReachable(testURL string) bool {
	// Parse URL to extract hostname and determine port
	parsedURL, err := url.Parse(testURL)
	if err != nil {
		return false
	}

	// Get hostname and add appropriate port
	hostname := parsedURL.Hostname()
	var port string
	switch parsedURL.Scheme {
	case "https":
		port = "443"
	case "http":
		port = "80"
	default:
		return false // unsupported scheme
	}

	// Create host:port for dialing
	hostPort := hostname + ":" + port

	// Test TCP connectivity
	conn, err := net.DialTimeout("tcp", hostPort, 5*time.Second)
	if conn != nil {
		_ = conn.Close()
	}
	return err == nil
}
