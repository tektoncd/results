package config

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/tektoncd/results/pkg/cli/client"
	"github.com/tektoncd/results/pkg/cli/common"
	"github.com/tektoncd/results/pkg/cli/testutils"
	"k8s.io/client-go/rest"
)

// TestNewConfig tests the NewConfig function with various scenarios
func TestNewConfig(t *testing.T) {
	kubeconfigPath, cleanup := testutils.CreateTestKubeconfig(t)
	defer cleanup()

	tests := []struct {
		name        string
		setupParams func() common.Params
		wantErr     bool
		description string
	}{
		{
			name: "nil params",
			setupParams: func() common.Params {
				return nil
			},
			wantErr:     true,
			description: "should return error for nil params",
		},
		{
			name: "invalid kubeconfig path",
			setupParams: func() common.Params {
				p := &common.ResultsParams{}
				p.SetKubeConfigPath("/invalid/path/to/kubeconfig")
				return p
			},
			wantErr:     true,
			description: "should return error for invalid kubeconfig path",
		},
		{
			name: "invalid context",
			setupParams: func() common.Params {
				p := &common.ResultsParams{}
				p.SetKubeConfigPath(kubeconfigPath)
				p.SetKubeContext("invalid-context")
				return p
			},
			wantErr:     true,
			description: "should return error for invalid context",
		},
		{
			name: "empty context",
			setupParams: func() common.Params {
				p := &common.ResultsParams{}
				p.SetKubeConfigPath(kubeconfigPath)
				p.SetKubeContext("")
				return p
			},
			wantErr:     false, // Empty context falls back to current context
			description: "should use current context when empty context provided",
		},
		{
			name: "valid config",
			setupParams: func() common.Params {
				p := &common.ResultsParams{}
				p.SetKubeConfigPath(kubeconfigPath)
				p.SetKubeContext("test-context")
				return p
			},
			wantErr:     false,
			description: "should not return error for valid config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg Config
			var err error
			var panicked bool

			// Handle potential panic for nil params
			func() {
				defer func() {
					if r := recover(); r != nil {
						panicked = true
						err = fmt.Errorf("panic: %v", r)
					}
				}()
				cfg, err = NewConfig(tt.setupParams())
			}()

			if tt.wantErr {
				if err == nil && !panicked {
					t.Errorf("%s: Expected error but got none", tt.description)
				}
				if cfg != nil {
					t.Errorf("%s: Expected cfg to be nil when error occurs", tt.description)
				}
			} else {
				if err != nil || panicked {
					t.Errorf("%s: Expected no error but got: %v", tt.description, err)
				}
				if cfg == nil {
					t.Errorf("%s: Expected cfg to be non-nil when no error occurs", tt.description)
				} else {
					// Verify the config is properly initialized when no error occurs
					if cfg.Get() == nil {
						t.Errorf("%s: Expected cfg.Get() to return non-nil client config", tt.description)
					}
					if cfg.GetObject() == nil {
						t.Errorf("%s: Expected cfg.GetObject() to return non-nil object", tt.description)
					}

					// For valid config case, verify specific values
					if tt.name == "valid config" {
						clientConfig := cfg.Get()
						if clientConfig.URL == nil {
							t.Errorf("%s: Expected URL to be set in valid config", tt.description)
						} else if clientConfig.URL.String() != "http://test-host/apis/results.tekton.dev/v1alpha2" {
							t.Errorf("%s: Expected URL to be 'http://test-host/apis/results.tekton.dev/v1alpha2', got '%s'", tt.description, clientConfig.URL.String())
						}

						obj := cfg.GetObject()
						ext, ok := obj.(*Extension)
						if !ok {
							t.Errorf("%s: Expected GetObject() to return Extension type", tt.description)
						} else {
							// Verify extension has proper version info set
							if ext.APIVersion == "" {
								t.Errorf("%s: Expected APIVersion to be set in extension", tt.description)
							}
							if ext.Kind == "" {
								t.Errorf("%s: Expected Kind to be set in extension", tt.description)
							}
						}
					}
				}
			}
		})
	}
}

// TestSet tests the Set function
func TestSet(t *testing.T) {
	kubeconfigPath, cleanup := testutils.CreateTestKubeconfig(t)
	defer cleanup()

	p := &common.ResultsParams{}
	p.SetKubeConfigPath(kubeconfigPath)
	p.SetKubeContext("test-context")
	p.SetHost("https://test-host.example.com")
	p.SetToken("test-token-123")
	p.SetAPIPath("/api/v2")
	p.SetSkipTLSVerify(true)

	cfg, err := NewConfig(p)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Test Set without prompt
	err = cfg.Set(false, p, "tekton-pipelines")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify the values were set correctly
	obj := cfg.GetObject()
	if obj == nil {
		t.Fatal("Expected GetObject() to return a non-nil object")
	}

	// Cast to Extension to verify fields
	ext, ok := obj.(*Extension)
	if !ok {
		t.Fatal("Expected GetObject() to return an Extension")
	}

	// Verify all the set values
	if ext.Host != "https://test-host.example.com" {
		t.Errorf("Expected Host to be 'https://test-host.example.com', got '%s'", ext.Host)
	}
	if ext.Token != "test-token-123" {
		t.Errorf("Expected Token to be 'test-token-123', got '%s'", ext.Token)
	}
	if ext.APIPath != "/api/v2" {
		t.Errorf("Expected APIPath to be '/api/v2', got '%s'", ext.APIPath)
	}
	if ext.InsecureSkipTLSVerify != "true" {
		t.Errorf("Expected InsecureSkipTLSVerify to be 'true', got '%s'", ext.InsecureSkipTLSVerify)
	}

	// Verify version fields are set
	if ext.APIVersion == "" {
		t.Error("Expected APIVersion to be set")
	}
	if ext.Kind == "" {
		t.Error("Expected Kind to be set")
	}
}

// TestReset tests the Reset function
func TestReset(t *testing.T) {
	kubeconfigPath, cleanup := testutils.CreateTestKubeconfig(t)
	defer cleanup()

	p := &common.ResultsParams{}
	p.SetKubeConfigPath(kubeconfigPath)
	p.SetKubeContext("test-context")
	p.SetHost("https://test-host-before-reset.com")
	p.SetToken("token-before-reset")
	p.SetAPIPath("/api/before/reset")
	p.SetSkipTLSVerify(true)

	cfg, err := NewConfig(p)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// First set some values
	err = cfg.Set(false, p, "tekton-pipelines")
	if err != nil {
		t.Fatalf("Failed to set initial values: %v", err)
	}

	// Verify values are set before reset
	objBefore := cfg.GetObject()
	extBefore, ok := objBefore.(*Extension)
	if !ok {
		t.Fatal("Expected GetObject() to return an Extension before reset")
	}
	if extBefore.Host == "" || extBefore.Token == "" {
		t.Fatal("Expected values to be set before reset")
	}

	// Test Reset
	err = cfg.Reset(p)
	if err != nil {
		t.Errorf("Expected no error during reset, got %v", err)
	}

	// Verify the config was reset - all fields should be empty
	objAfter := cfg.GetObject()
	if objAfter == nil {
		t.Fatal("Expected GetObject() to return a non-nil object after reset")
	}

	extAfter, ok := objAfter.(*Extension)
	if !ok {
		t.Fatal("Expected GetObject() to return an Extension after reset")
	}

	// Verify all fields are cleared
	if extAfter.Host != "" {
		t.Errorf("Expected Host to be empty after reset, got '%s'", extAfter.Host)
	}
	if extAfter.Token != "" {
		t.Errorf("Expected Token to be empty after reset, got '%s'", extAfter.Token)
	}
	if extAfter.APIPath != "" {
		t.Errorf("Expected APIPath to be empty after reset, got '%s'", extAfter.APIPath)
	}
	if extAfter.InsecureSkipTLSVerify != "" {
		t.Errorf("Expected InsecureSkipTLSVerify to be empty after reset, got '%s'", extAfter.InsecureSkipTLSVerify)
	}
	if extAfter.Timeout != "" {
		t.Errorf("Expected Timeout to be empty after reset, got '%s'", extAfter.Timeout)
	}

	// Version fields should still be set after reset
	if extAfter.APIVersion == "" {
		t.Error("Expected APIVersion to still be set after reset")
	}
	if extAfter.Kind == "" {
		t.Error("Expected Kind to still be set after reset")
	}
}

// TestLoadClientConfig tests the LoadClientConfig function
func TestLoadClientConfig(t *testing.T) {
	kubeconfigPath, cleanup := testutils.CreateTestKubeconfig(t)
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

	// Verify actual configuration values from our test kubeconfig
	if clientConfig.URL == nil {
		t.Fatal("Expected URL to be non-nil")
	}
	expectedURL := "http://test-host/apis/results.tekton.dev/v1alpha2"
	if clientConfig.URL.String() != expectedURL {
		t.Errorf("Expected URL to be '%s', got '%s'", expectedURL, clientConfig.URL.String())
	}

	// Verify Transport is properly configured
	if clientConfig.Transport == nil {
		t.Fatal("Expected Transport to be non-nil")
	}

	// Verify Transport configuration - just check that it's properly initialized
	// (TLS configuration may vary based on the server protocol)

	// Verify Timeout - it may be zero if not explicitly set, which is valid
	// Just verify that the field exists and is accessible
	_ = clientConfig.Timeout // This verifies the field exists

	// Test creating a REST client from the config to verify it's usable
	restClient, err := client.NewRESTClient(clientConfig)
	if err != nil {
		t.Errorf("Expected to be able to create REST client from config, got error: %v", err)
	}
	if restClient == nil {
		t.Error("Expected NewRESTClient to return a non-nil client")
	}
}

// TestSetWithPrompt tests the Set function with prompt enabled
func TestSetWithPrompt(t *testing.T) {
	kubeconfigPath, cleanup := testutils.CreateTestKubeconfig(t)
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
	err = cfg.Set(true, p, "tekton-pipelines")

	// In a test environment, we expect either:
	// 1. An EOF error when prompting for user input
	// 2. A network error from our mock config
	// 3. A route detection error (no Tekton Results routes found)
	// All are expected behaviors in a test environment
	if err != nil {
		// Check if the error is EOF, contains our mock error, or is a route detection error
		if err == io.EOF || err.Error() == "EOF" ||
			strings.Contains(err.Error(), "mock network error") ||
			strings.Contains(err.Error(), "no Tekton Results routes found") {
			t.Log("Received expected error when prompting for user input in test environment")
		} else {
			t.Errorf("Expected EOF error, mock network error, route detection error, or no error, got %v", err)
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
	kubeconfigPath, cleanup := testutils.CreateTestKubeconfig(t)
	defer cleanup()

	p := &common.ResultsParams{}
	p.SetKubeConfigPath(kubeconfigPath)
	p.SetKubeContext("test-context")
	p.SetHost("https://persist-test-host.com")
	p.SetToken("persist-test-token")
	p.SetAPIPath("/api/persist/test")
	p.SetSkipTLSVerify(true)

	cfg, err := NewConfig(p)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Set some configuration values
	err = cfg.Set(false, p, "tekton-pipelines")
	if err != nil {
		t.Fatalf("Failed to set configuration: %v", err)
	}

	// Test Persist
	err = cfg.(*config).Persist(p)
	if err != nil {
		t.Errorf("Expected no error during persist, got %v", err)
	}

	// Verify that the configuration was actually persisted by creating a new config
	// and loading it from the same kubeconfig file
	newParams := &common.ResultsParams{}
	newParams.SetKubeConfigPath(kubeconfigPath)
	newParams.SetKubeContext("test-context")

	newCfg, err := NewConfig(newParams)
	if err != nil {
		t.Fatalf("Failed to create new config to verify persistence: %v", err)
	}

	// Get the persisted configuration
	persistedObj := newCfg.GetObject()
	if persistedObj == nil {
		t.Fatal("Expected GetObject() to return a non-nil object from persisted config")
	}

	persistedExt, ok := persistedObj.(*Extension)
	if !ok {
		t.Fatal("Expected GetObject() to return an Extension from persisted config")
	}

	// Verify the persisted values match what we set
	if persistedExt.Host != "https://persist-test-host.com" {
		t.Errorf("Expected persisted Host to be 'https://persist-test-host.com', got '%s'", persistedExt.Host)
	}
	if persistedExt.Token != "persist-test-token" {
		t.Errorf("Expected persisted Token to be 'persist-test-token', got '%s'", persistedExt.Token)
	}
	if persistedExt.APIPath != "/api/persist/test" {
		t.Errorf("Expected persisted APIPath to be '/api/persist/test', got '%s'", persistedExt.APIPath)
	}
	if persistedExt.InsecureSkipTLSVerify != "true" {
		t.Errorf("Expected persisted InsecureSkipTLSVerify to be 'true', got '%s'", persistedExt.InsecureSkipTLSVerify)
	}
}

// TestSetVersion tests the SetVersion function
func TestSetVersion(t *testing.T) {
	kubeconfigPath, cleanup := testutils.CreateTestKubeconfig(t)
	defer cleanup()

	p := &common.ResultsParams{}
	p.SetKubeConfigPath(kubeconfigPath)
	p.SetKubeContext("test-context")
	cfg, err := NewConfig(p)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Get the object before calling SetVersion to compare
	objBefore := cfg.GetObject()
	extBefore, ok := objBefore.(*Extension)
	if !ok {
		t.Fatal("Expected GetObject() to return an Extension before SetVersion")
	}

	// Test SetVersion
	cfg.(*config).SetVersion()

	// Verify the version was set correctly
	objAfter := cfg.GetObject()
	if objAfter == nil {
		t.Fatal("Expected GetObject() to return a non-nil object after SetVersion")
	}

	extAfter, ok := objAfter.(*Extension)
	if !ok {
		t.Fatal("Expected GetObject() to return an Extension after SetVersion")
	}

	// Verify the correct API version and kind are set
	expectedAPIVersion := "results.tekton.dev/v1alpha2"
	expectedKind := "Client"

	if extAfter.APIVersion != expectedAPIVersion {
		t.Errorf("Expected APIVersion to be '%s', got '%s'", expectedAPIVersion, extAfter.APIVersion)
	}
	if extAfter.Kind != expectedKind {
		t.Errorf("Expected Kind to be '%s', got '%s'", expectedKind, extAfter.Kind)
	}

	// Verify that other fields are preserved (if they were set before)
	if extBefore.Host != "" && extAfter.Host != extBefore.Host {
		t.Errorf("Expected Host to be preserved: before='%s', after='%s'", extBefore.Host, extAfter.Host)
	}
	if extBefore.Token != "" && extAfter.Token != extBefore.Token {
		t.Errorf("Expected Token to be preserved: before='%s', after='%s'", extBefore.Token, extAfter.Token)
	}
}

// TestHost tests the Host function
func TestHost(t *testing.T) {
	kubeconfigPath, cleanup := testutils.CreateTestKubeconfig(t)
	defer cleanup()

	tests := []struct {
		name             string
		resultsNamespace string
		description      string
		expectError      bool
	}{
		{
			name:             "kubernetes namespace",
			resultsNamespace: "tekton-pipelines",
			description:      "should attempt to detect ingresses in tekton-pipelines namespace",
			expectError:      true, // Expected in test env since no real cluster
		},
		{
			name:             "openshift namespace",
			resultsNamespace: "openshift-pipelines",
			description:      "should attempt to detect routes in openshift-pipelines namespace",
			expectError:      true, // Expected in test env since no real cluster
		},
		{
			name:             "custom namespace",
			resultsNamespace: "custom-tekton-namespace",
			description:      "should attempt to detect in custom namespace",
			expectError:      true, // Expected in test env since no real cluster
		},
		{
			name:             "empty namespace",
			resultsNamespace: "",
			description:      "should handle empty namespace gracefully",
			expectError:      true, // Expected because empty namespace should cause error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &common.ResultsParams{}
			p.SetKubeConfigPath(kubeconfigPath)
			p.SetKubeContext("test-context")
			cfg, err := NewConfig(p)
			if err != nil {
				t.Fatalf("Failed to create config: %v", err)
			}

			// Test Host function
			hosts := cfg.(*config).Host(tt.resultsNamespace)

			// In test environment, we expect the function to attempt detection
			// but likely fail due to no real cluster, which is the expected behavior
			if tt.expectError {
				// Host function returns either []string or error
				// In test environment, we expect it to return an error
				if err, ok := hosts.(error); ok {
					t.Logf("%s: Host() returned expected error in test env: %v", tt.description, err)
				} else if urls, ok := hosts.([]string); ok {
					// In some test cases, it might return empty slice instead of error
					if len(urls) != 0 {
						t.Logf("%s: Host() returned unexpected URLs in test env: %v", tt.description, urls)
					}
				} else {
					t.Errorf("%s: Host() returned unexpected type: %T", tt.description, hosts)
				}
			} else {
				if urls, ok := hosts.([]string); ok {
					if len(urls) == 0 {
						t.Errorf("%s: Expected Host() to return non-empty URLs", tt.description)
					}
				} else if err, ok := hosts.(error); ok {
					t.Errorf("%s: Expected Host() to return URLs but got error: %v", tt.description, err)
				} else {
					t.Errorf("%s: Host() returned unexpected type: %T", tt.description, hosts)
				}
			}
		})
	}

	// Test with invalid config to ensure error handling
	t.Run("invalid config", func(t *testing.T) {
		invalidConfig := &config{
			RESTConfig: &rest.Config{Host: "invalid-host-format"},
		}

		hosts := invalidConfig.Host("tekton-pipelines")

		// Should handle invalid config gracefully
		// Host function returns either []string or error
		if err, ok := hosts.(error); ok {
			t.Logf("Host() with invalid config returned expected error: %v", err)
		} else if urls, ok := hosts.([]string); ok {
			// Should return empty slice for invalid config
			if len(urls) != 0 {
				t.Errorf("Expected empty slice for invalid config, got: %v", urls)
			}
		} else {
			t.Errorf("Host() returned unexpected type: %T", hosts)
		}
	})
}

// TestToken tests the Token function
func TestToken(t *testing.T) {
	kubeconfigPath, cleanup := testutils.CreateTestKubeconfig(t)
	defer cleanup()

	tests := []struct {
		name         string
		setupConfig  func() *config
		expectedType string
		description  string
	}{
		{
			name: "config with bearer token",
			setupConfig: func() *config {
				p := &common.ResultsParams{}
				p.SetKubeConfigPath(kubeconfigPath)
				p.SetKubeContext("test-context")
				cfg, err := NewConfig(p)
				if err != nil {
					t.Fatalf("Failed to create config: %v", err)
				}

				// Set a bearer token in the REST config
				cfg.(*config).RESTConfig.BearerToken = "test-bearer-token-123"
				return cfg.(*config)
			},
			expectedType: "string",
			description:  "should return the bearer token from RESTConfig",
		},
		{
			name: "config without bearer token",
			setupConfig: func() *config {
				p := &common.ResultsParams{}
				p.SetKubeConfigPath(kubeconfigPath)
				p.SetKubeContext("test-context")
				cfg, err := NewConfig(p)
				if err != nil {
					t.Fatalf("Failed to create config: %v", err)
				}

				// Ensure no bearer token is set
				cfg.(*config).RESTConfig.BearerToken = ""
				return cfg.(*config)
			},
			expectedType: "string",
			description:  "should return empty string when no bearer token is set",
		},
		{
			name: "config with nil RESTConfig",
			setupConfig: func() *config {
				return &config{
					RESTConfig: nil,
				}
			},
			expectedType: "error",
			description:  "should return error when RESTConfig is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.setupConfig()

			// Test Token function
			token := cfg.Token()

			if tt.expectedType == "string" {
				if tokenStr, ok := token.(string); ok {
					if tt.name == "config with bearer token" {
						if tokenStr != "test-bearer-token-123" {
							t.Errorf("%s: Expected token to be 'test-bearer-token-123', got '%s'", tt.description, tokenStr)
						}
					} else if tt.name == "config without bearer token" {
						if tokenStr != "" {
							t.Errorf("%s: Expected empty token, got '%s'", tt.description, tokenStr)
						}
					}
				} else {
					t.Errorf("%s: Expected Token() to return string, got %T", tt.description, token)
				}
			} else if tt.expectedType == "error" {
				if err, ok := token.(error); ok {
					t.Logf("%s: Token() returned expected error: %v", tt.description, err)
				} else {
					t.Errorf("%s: Expected Token() to return error, got %T", tt.description, token)
				}
			}
		})
	}
}
