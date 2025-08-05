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

	// Check for OpenShift-specific API groups using a map for efficient lookup
	openShiftAPIGroups := map[string]bool{
		"route.openshift.io":    true, // Routes (core OpenShift feature)
		"image.openshift.io":    true, // Image streams
		"apps.openshift.io":     true, // DeploymentConfigs
		"security.openshift.io": true, // Security Context Constraints
		"project.openshift.io":  true, // Projects
		"user.openshift.io":     true, // Users and groups
		"oauth.openshift.io":    true, // OAuth
		"config.openshift.io":   true, // Cluster configuration
	}

	apiGroupList, err := discoveryClient.ServerGroups()
	if err != nil {
		return PlatformUnknown
	}

	// Check if any OpenShift API groups are present
	for _, group := range apiGroupList.Groups {
		if openShiftAPIGroups[group.Name] {
			return PlatformOpenShift
		}
	}

	// No OpenShift API groups found - this is Kubernetes
	return PlatformKubernetes
}
