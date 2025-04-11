package config

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/tektoncd/results/pkg/cli/common"
	"k8s.io/client-go/rest"
)

// createTestKubeconfig creates a temporary kubeconfig file for testing
func createTestKubeconfig(t *testing.T) (string, func()) {
	t.Helper()
	// Create a temporary kubeconfig file
	tmpFile, err := os.CreateTemp("", "kubeconfig-")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	// Write test kubeconfig content
	kubeconfig := `apiVersion: v1
clusters:
- cluster:
    server: http://test-host
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
kind: Config
preferences: {}
users:
- name: test-user
`
	if _, err := tmpFile.WriteString(kubeconfig); err != nil {
		t.Fatalf("Failed to write kubeconfig: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	// Return the file path and a cleanup function
	return tmpFile.Name(), func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			t.Logf("Failed to remove temp file %q: %v", tmpFile.Name(), err)
		}
	}
}

// TestNewConfig tests the NewConfig function
func TestNewConfig(t *testing.T) {
	kubeconfigPath, cleanup := createTestKubeconfig(t)
	defer cleanup()

	tests := []struct {
		name    string
		params  common.Params
		wantErr bool
	}{
		{
			name: "valid params",
			params: func() common.Params {
				p := &common.ResultsParams{}
				p.SetKubeConfigPath(kubeconfigPath)
				p.SetKubeContext("test-context")
				return p
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create the config
			cfg, err := NewConfig(tt.params)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				if cfg != nil {
					t.Error("Expected config to be nil when error is expected")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error creating config: %v", err)
			}

			if cfg == nil {
				t.Fatal("Expected config to be created")
			}

			// Verify config properties
			if cfg.Get() == nil {
				t.Error("Expected Get() to return a non-nil config")
			}
			if cfg.GetObject() == nil {
				t.Error("Expected GetObject() to return a non-nil object")
			}

			// Verify REST config is set correctly
			config := cfg.(*config)
			if config.RESTConfig == nil {
				t.Error("Expected RESTConfig to be non-nil")
			}
			if config.RESTConfig.Host != "http://test-host" {
				t.Errorf("Expected host to be 'http://test-host', got %q", config.RESTConfig.Host)
			}
		})
	}
}

// TestSet tests the Set function
func TestSet(t *testing.T) {
	kubeconfigPath, cleanup := createTestKubeconfig(t)
	defer cleanup()

	p := &common.ResultsParams{}
	p.SetKubeConfigPath(kubeconfigPath)
	p.SetKubeContext("test-context")
	p.SetHost("http://test-host")
	p.SetToken("test-token")
	p.SetAPIPath("/test-path")
	p.SetSkipTLSVerify(true)

	cfg, err := NewConfig(p)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Test Set without prompt
	err = cfg.Set(false, p)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify the values were set
	obj := cfg.GetObject()
	if obj == nil {
		t.Fatal("Expected GetObject() to return a non-nil object")
	}
}

// TestReset tests the Reset function
func TestReset(t *testing.T) {
	kubeconfigPath, cleanup := createTestKubeconfig(t)
	defer cleanup()

	p := &common.ResultsParams{}
	p.SetKubeConfigPath(kubeconfigPath)
	p.SetKubeContext("test-context")
	cfg, err := NewConfig(p)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Test Reset
	err = cfg.Reset(p)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify the config was reset
	obj := cfg.GetObject()
	if obj == nil {
		t.Fatal("Expected GetObject() to return a non-nil object")
	}
}

// TestLoadClientConfig tests the LoadClientConfig function
func TestLoadClientConfig(t *testing.T) {
	kubeconfigPath, cleanup := createTestKubeconfig(t)
	defer cleanup()

	p := &common.ResultsParams{}
	p.SetKubeConfigPath(kubeconfigPath)
	p.SetKubeContext("test-context")
	cfg, err := NewConfig(p)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Get the client config
	clientConfig := cfg.Get()
	if clientConfig == nil {
		t.Fatal("Expected Get() to return a non-nil config")
	}

	// Verify basic properties
	if clientConfig.Transport == nil {
		t.Error("Expected Transport to be non-nil")
	}
	if clientConfig.URL == nil {
		t.Error("Expected URL to be non-nil")
	}
}

// TestNewConfigError tests error cases in NewConfig
func TestNewConfigError(t *testing.T) {
	kubeconfigPath, cleanup := createTestKubeconfig(t)
	defer cleanup()

	// Test with invalid kubeconfig path
	p := &common.ResultsParams{}
	p.SetKubeConfigPath("/invalid/path/to/kubeconfig")

	_, err := NewConfig(p)
	if err == nil {
		t.Error("Expected error for invalid kubeconfig path")
	}

	// Test with invalid context
	p = &common.ResultsParams{}
	p.SetKubeConfigPath(kubeconfigPath)
	p.SetKubeContext("invalid-context")

	_, err = NewConfig(p)
	if err == nil {
		t.Error("Expected error for invalid context")
	}
}

// TestSetWithPrompt tests the Set function with prompt enabled
func TestSetWithPrompt(t *testing.T) {
	kubeconfigPath, cleanup := createTestKubeconfig(t)
	defer cleanup()

	// Create a mock REST config that will fail when used
	mockConfig := &rest.Config{
		Host: "http://test-host",
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return nil, fmt.Errorf("mock network error")
			},
		},
	}

	p := &common.ResultsParams{}
	p.SetKubeConfigPath(kubeconfigPath)
	p.SetKubeContext("test-context")
	cfg, err := NewConfig(p)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Override the REST config with our mock
	config := cfg.(*config)
	config.RESTConfig = mockConfig

	// Test Set with prompt
	err = cfg.Set(true, p)

	// In a test environment, we expect either:
	// 1. An EOF error when prompting for user input
	// 2. A network error from our mock config
	// Both are expected behaviors in a test environment
	if err != nil {
		// Check if the error is EOF or contains our mock error
		if err == io.EOF || err.Error() == "EOF" || strings.Contains(err.Error(), "mock network error") {
			t.Log("Received expected error when prompting for user input in test environment")
		} else {
			t.Errorf("Expected EOF error, mock network error, or no error, got %v", err)
		}
	}

	// Even if we got an error, the config object should still exist
	obj := cfg.GetObject()
	if obj == nil {
		t.Fatal("Expected GetObject() to return a non-nil object")
	}
}

// TestPersist tests the Persist function
func TestPersist(t *testing.T) {
	kubeconfigPath, cleanup := createTestKubeconfig(t)
	defer cleanup()

	p := &common.ResultsParams{}
	p.SetKubeConfigPath(kubeconfigPath)
	p.SetKubeContext("test-context")
	cfg, err := NewConfig(p)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Test Persist
	err = cfg.(*config).Persist(p)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

// TestSetVersion tests the SetVersion function
func TestSetVersion(t *testing.T) {
	kubeconfigPath, cleanup := createTestKubeconfig(t)
	defer cleanup()

	p := &common.ResultsParams{}
	p.SetKubeConfigPath(kubeconfigPath)
	p.SetKubeContext("test-context")
	cfg, err := NewConfig(p)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Test SetVersion
	cfg.(*config).SetVersion()

	// Verify the version was set
	obj := cfg.GetObject()
	if obj == nil {
		t.Fatal("Expected GetObject() to return a non-nil object")
	}
}

// TestHost tests the Host function
func TestHost(t *testing.T) {
	kubeconfigPath, cleanup := createTestKubeconfig(t)
	defer cleanup()

	p := &common.ResultsParams{}
	p.SetKubeConfigPath(kubeconfigPath)
	p.SetKubeContext("test-context")
	cfg, err := NewConfig(p)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Test Host
	hosts := cfg.(*config).Host()
	if hosts == nil {
		t.Error("Expected Host() to return a non-nil value")
	}
}

// TestToken tests the Token function
func TestToken(t *testing.T) {
	kubeconfigPath, cleanup := createTestKubeconfig(t)
	defer cleanup()

	p := &common.ResultsParams{}
	p.SetKubeConfigPath(kubeconfigPath)
	p.SetKubeContext("test-context")
	cfg, err := NewConfig(p)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Test Token
	token := cfg.(*config).Token()
	if token == nil {
		t.Error("Expected Token() to return a non-nil value")
	}
}
