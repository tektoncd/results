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
			expectedType: PlatformUnknown,
			description:  "should return Unknown when discovery client creation fails",
		},
		{
			name:         "empty host config",
			config:       &rest.Config{Host: ""},
			expectedType: PlatformUnknown,
			description:  "should return Unknown when host is empty",
		},
		{
			name:         "unreachable host config",
			config:       &rest.Config{Host: "https://unreachable.cluster:6443"},
			expectedType: PlatformUnknown,
			description:  "should return Unknown when host is unreachable",
		},
		{
			name:         "valid config but no cluster",
			config:       &rest.Config{Host: "https://kubernetes.local:6443"},
			expectedType: PlatformUnknown,
			description:  "should return Unknown in test environment (no real cluster)",
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
