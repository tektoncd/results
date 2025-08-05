package config

import (
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

// PlatformType represents the type of Kubernetes platform
type PlatformType string

// Platform types for identifying the underlying Kubernetes platform
const (
	PlatformUnknown    PlatformType = "Unknown"
	PlatformOpenShift  PlatformType = "OpenShift"
	PlatformKubernetes PlatformType = "Kubernetes"
)

// DetectPlatform determines if we're running on OpenShift or Kubernetes
// Checks for OpenShift-specific API groups
func DetectPlatform(c *rest.Config) PlatformType {
	if c == nil {
		return PlatformUnknown
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(c)
	if err != nil {
		return PlatformKubernetes // Default to Kubernetes if discovery fails
	}

	// Check for OpenShift-specific API groups
	openShiftAPIGroups := []string{
		"route.openshift.io",    // Routes (core OpenShift feature)
		"image.openshift.io",    // Image streams
		"apps.openshift.io",     // DeploymentConfigs
		"security.openshift.io", // Security Context Constraints
		"project.openshift.io",  // Projects
		"user.openshift.io",     // Users and groups
		"oauth.openshift.io",    // OAuth
		"config.openshift.io",   // Cluster configuration
	}

	apiGroupList, err := discoveryClient.ServerGroups()
	if err != nil {
		return PlatformKubernetes // Default to Kubernetes if API discovery fails
	}

	availableGroups := make(map[string]bool)
	for _, group := range apiGroupList.Groups {
		availableGroups[group.Name] = true
	}

	// Check if any OpenShift API groups are present
	for _, osGroup := range openShiftAPIGroups {
		if _, found := availableGroups[osGroup]; found {
			return PlatformOpenShift
		}
	}

	// No OpenShift API groups found - this is Kubernetes
	return PlatformKubernetes
}

// GetDefaultResultsNamespace returns the default namespace based on platform
func GetDefaultResultsNamespace(platform PlatformType) string {
	switch platform {
	case PlatformOpenShift:
		return "openshift-pipelines"
	case PlatformKubernetes:
		return "tekton-pipelines"
	default:
		return "tekton-pipelines" // default to K8s
	}
}
