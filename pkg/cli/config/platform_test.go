package config

import (
	"testing"

	"k8s.io/client-go/rest"
)

// TestDetectPlatform tests the DetectPlatform function
func TestDetectPlatform(t *testing.T) {
	tests := []struct {
		name         string
		config       *rest.Config
		expectedType PlatformType
		description  string
	}{
		{
			name:         "nil config",
			config:       nil,
			expectedType: PlatformUnknown,
			description:  "should return Unknown for nil config",
		},
		{
			name:         "invalid host config",
			config:       &rest.Config{Host: "invalid-host-format"},
			expectedType: PlatformKubernetes,
			description:  "should default to Kubernetes when discovery client creation fails",
		},
		{
			name:         "empty host config",
			config:       &rest.Config{Host: ""},
			expectedType: PlatformKubernetes,
			description:  "should default to Kubernetes when host is empty",
		},
		{
			name:         "unreachable host config",
			config:       &rest.Config{Host: "https://unreachable.cluster:6443"},
			expectedType: PlatformKubernetes,
			description:  "should default to Kubernetes when host is unreachable",
		},
		{
			name:         "valid config but no cluster",
			config:       &rest.Config{Host: "https://kubernetes.local:6443"},
			expectedType: PlatformKubernetes,
			description:  "should default to Kubernetes in test environment (no real cluster)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectPlatform(tt.config)
			if result != tt.expectedType {
				t.Errorf("%s: DetectPlatform() = %v, want %v", tt.description, result, tt.expectedType)
			}
		})
	}
}

// TestGetDefaultResultsNamespace tests the GetDefaultResultsNamespace function
func TestGetDefaultResultsNamespace(t *testing.T) {
	tests := []struct {
		name              string
		platform          PlatformType
		expectedNamespace string
		description       string
	}{
		{
			name:              "openshift platform",
			platform:          PlatformOpenShift,
			expectedNamespace: "openshift-pipelines",
			description:       "should return openshift-pipelines for OpenShift platform",
		},
		{
			name:              "kubernetes platform",
			platform:          PlatformKubernetes,
			expectedNamespace: "tekton-pipelines",
			description:       "should return tekton-pipelines for Kubernetes platform",
		},
		{
			name:              "unknown platform",
			platform:          PlatformUnknown,
			expectedNamespace: "tekton-pipelines",
			description:       "should return tekton-pipelines for Unknown platform (defaults to Kubernetes)",
		},
		{
			name:              "invalid platform",
			platform:          PlatformType("InvalidPlatform"),
			expectedNamespace: "tekton-pipelines",
			description:       "should return tekton-pipelines for invalid platform (defaults to Kubernetes)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetDefaultResultsNamespace(tt.platform)
			if result != tt.expectedNamespace {
				t.Errorf("%s: GetDefaultResultsNamespace(%v) = %v, want %v",
					tt.description, tt.platform, result, tt.expectedNamespace)
			}
		})
	}
}

// TestPlatformTypeConstants tests that the platform type constants have correct values
func TestPlatformTypeConstants(t *testing.T) {
	tests := []struct {
		name           string
		platform       PlatformType
		expectedString string
	}{
		{
			name:           "PlatformUnknown constant",
			platform:       PlatformUnknown,
			expectedString: "Unknown",
		},
		{
			name:           "PlatformOpenShift constant",
			platform:       PlatformOpenShift,
			expectedString: "OpenShift",
		},
		{
			name:           "PlatformKubernetes constant",
			platform:       PlatformKubernetes,
			expectedString: "Kubernetes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.platform) != tt.expectedString {
				t.Errorf("Platform constant %s should equal %q, got %q",
					tt.name, tt.expectedString, string(tt.platform))
			}
		})
	}
}
