package config

import (
	"strings"
	"testing"

	"github.com/tektoncd/results/pkg/cli/common"
	"github.com/tektoncd/results/pkg/cli/config"
	"github.com/tektoncd/results/pkg/cli/testutils"
	"k8s.io/client-go/tools/clientcmd"
)

// TestSetCommandPromptBehavior tests prompting behavior using ExecuteCommand with stdin simulation
func TestSetCommandPromptBehavior(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		stdinInput   string
		expectPrompt bool
		expectError  bool
		description  string
	}{
		{
			name:         "no flags should prompt for host",
			args:         []string{"set"},
			stdinInput:   "", // Empty input should cause error since no host provided
			expectPrompt: true,
			expectError:  true,
			description:  "when no configuration flags are provided, should prompt for host",
		},
		{
			name:         "host provided should not prompt",
			args:         []string{"set", "--host=https://example.com", "--token=test-token"},
			stdinInput:   "", // No input needed
			expectPrompt: false,
			expectError:  false,
			description:  "when host and token are provided, should not prompt",
		},
		{
			name:         "token provided should not prompt",
			args:         []string{"set", "--token=test-token", "--host=https://example.com"},
			stdinInput:   "", // No input needed
			expectPrompt: false,
			expectError:  false,
			description:  "when token and host are provided, should not prompt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary kubeconfig
			kubeconfigPath := testutils.CreateTestKubeconfig(t, "")

			// Create test params
			params := &common.ResultsParams{}
			params.SetKubeConfigPath(kubeconfigPath)
			params.SetKubeContext("test-context")

			// Get the command
			cmd := setCommand(params)

			// Set up stdin simulation if we expect prompting
			if tt.expectPrompt && tt.stdinInput != "" {
				cmd.SetIn(strings.NewReader(tt.stdinInput))
			}

			// Capture stdout/stderr
			var stdout, stderr strings.Builder
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)

			// Add kubeconfig to args and execute
			allArgs := append([]string{"--kubeconfig=" + kubeconfigPath}, tt.args...)
			cmd.SetArgs(allArgs)

			// Execute the command
			err := cmd.Execute()

			// Check error expectations
			if tt.expectError {
				if err == nil {
					t.Errorf("%s: expected error but got none", tt.description)
				}
				return
			}

			// Should not have error for success cases
			if err != nil {
				t.Errorf("%s: unexpected error: %v", tt.description, err)
				return
			}

			// Check prompt expectations by examining output
			output := stdout.String() + stderr.String()
			containsPrompt := strings.Contains(output, "Host:") || strings.Contains(output, "Token:") ||
				strings.Contains(output, "Please select") || strings.Contains(output, "Use the arrow keys")

			if tt.expectPrompt && !containsPrompt {
				t.Errorf("%s: expected prompting but found no prompt messages in output", tt.description)
			}

			if !tt.expectPrompt && containsPrompt {
				t.Errorf("%s: expected no prompting but found prompt messages in output", tt.description)
			}
		})
	}
}

// TestSetCommandExecution tests that the set command executes properly and persists configuration
func TestSetCommandExecution(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectError   bool
		expectedHost  string
		expectedToken string
		description   string
	}{
		{
			name:          "set with host and token",
			args:          []string{"--host=https://test-host.com", "--token=test-token"},
			expectError:   false,
			expectedHost:  "https://test-host.com",
			expectedToken: "test-token",
			description:   "should persist basic configuration",
		},
		{
			name:          "set with all flags",
			args:          []string{"--host=https://complete-host.com", "--token=complete-token", "--api-path=/v1", "--insecure-skip-tls-verify"},
			expectError:   false,
			expectedHost:  "https://complete-host.com",
			expectedToken: "complete-token",
			description:   "should persist complete configuration with all flags",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary kubeconfig
			kubeconfigPath := testutils.CreateTestKubeconfig(t, "")

			// Create test params
			params := &common.ResultsParams{}
			params.SetKubeConfigPath(kubeconfigPath)
			params.SetKubeContext("test-context")

			// Get command
			cmd := Command(params)

			// Add kubeconfig to args and execute
			allArgs := append([]string{"--kubeconfig=" + kubeconfigPath, "set"}, tt.args...)
			_, err := testutils.ExecuteCommand(cmd, allArgs...)

			// Check error expectations
			if tt.expectError {
				if err == nil {
					t.Errorf("%s: expected error but got none", tt.description)
				}
				return
			}

			// Should not have error for success cases
			if err != nil {
				t.Errorf("%s: unexpected error: %v", tt.description, err)
				return
			}

			// Verify the configuration was persisted
			var extension config.Extension
			found, err := testutils.ReadKubeconfigExtension(t, kubeconfigPath, config.ExtensionName, &extension)
			if err != nil {
				t.Fatalf("failed to read persisted config: %v", err)
			}
			if !found {
				t.Fatalf("expected configuration to be persisted, but extension not found")
			}

			// Basic verification that values were set
			if extension.Host == "" {
				t.Errorf("expected host to be set in persisted config")
			}
			if extension.Token == "" {
				t.Errorf("expected token to be set in persisted config")
			}
		})
	}
}

func TestSetCommandConfigOverwrite(t *testing.T) {
	// Create temporary kubeconfig
	kubeconfigPath := testutils.CreateTestKubeconfig(t, "")

	// Create command
	params := &common.ResultsParams{}
	params.SetKubeConfigPath(kubeconfigPath)
	params.SetKubeContext("test-context")
	cmd := Command(params)

	// First, set initial configuration
	args1 := []string{"--kubeconfig=" + kubeconfigPath, "set", "--host=https://first-host.com", "--token=first-token"}
	_, err := testutils.ExecuteCommand(cmd, args1...)
	if err != nil {
		t.Fatalf("failed to set initial config: %v", err)
	}

	// Verify first configuration was set
	var extension1 config.Extension
	found1, err := testutils.ReadKubeconfigExtension(t, kubeconfigPath, config.ExtensionName, &extension1)
	if err != nil {
		t.Fatalf("failed to read first extension: %v", err)
	}
	if !found1 || extension1.Host != "https://first-host.com" {
		t.Fatalf("First configuration not set correctly")
	}

	// Now overwrite with new configuration
	args2 := []string{"--kubeconfig=" + kubeconfigPath, "set", "--host=https://second-host.com", "--token=second-token", "--api-path=/v2"}
	_, err = testutils.ExecuteCommand(cmd, args2...)
	if err != nil {
		t.Fatalf("failed to overwrite config: %v", err)
	}

	// Verify second configuration overwrote the first
	var extension2 config.Extension
	found2, err := testutils.ReadKubeconfigExtension(t, kubeconfigPath, config.ExtensionName, &extension2)
	if err != nil {
		t.Fatalf("failed to read second config: %v", err)
	}
	if !found2 {
		t.Fatalf("expected second configuration to be persisted, but extension not found")
	}

	if extension2.Host != "https://second-host.com" {
		t.Errorf("expected host %q, got %q", "https://second-host.com", extension2.Host)
	}
	if extension2.Token != "second-token" {
		t.Errorf("expected token %q, got %q", "second-token", extension2.Token)
	}
	if extension2.APIPath != "/v2" {
		t.Errorf("expected api-path %q, got %q", "/v2", extension2.APIPath)
	}
}

// TestSetConfigDefaultNamespaceStorage tests that configuration is stored in the tekton-results-config context
func TestSetConfigDefaultNamespaceStorage(t *testing.T) {
	kubeconfigPath := testutils.CreateTestKubeconfig(t, "production")

	params := &common.ResultsParams{}
	cmd := Command(params)

	// Set configuration in production namespace context
	args := []string{
		"--kubeconfig=" + kubeconfigPath,
		"--context=test-context",
		"set",
		"--host=https://default-ns-test.com",
		"--token=default-ns-token",
		"--api-path=/api/default/test",
	}
	_, err := testutils.ExecuteCommand(cmd, args...)
	if err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Verify configuration is stored in tekton-results-config context, not current context
	configLoadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	apiConfig, err := configLoadingRules.Load()
	if err != nil {
		t.Fatalf("Failed to load kubeconfig: %v", err)
	}

	// Check that current context (production namespace) does NOT have extensions
	currentContext := apiConfig.Contexts["test-context"]
	if currentContext.Extensions != nil && currentContext.Extensions["tekton-results"] != nil {
		t.Error("Configuration should not be stored in current context extensions")
	}

	// Check that config context DOES have extensions
	configContextName := "tekton-results-config/test-cluster/test-user"
	configContext := apiConfig.Contexts[configContextName]
	if configContext == nil {
		t.Fatalf("Config context should exist: %s", configContextName)
	}
	if configContext.Extensions == nil || configContext.Extensions["tekton-results"] == nil {
		t.Error("Configuration should be stored in config context extensions")
	}

	// Verify config context properties
	if configContext.Cluster != "test-cluster" {
		t.Errorf("Config context cluster should be 'test-cluster', got '%s'", configContext.Cluster)
	}
	if configContext.AuthInfo != "test-user" {
		t.Errorf("Config context user should be 'test-user', got '%s'", configContext.AuthInfo)
	}
	if configContext.Namespace != "default" {
		t.Errorf("Config context namespace should be 'default', got '%s'", configContext.Namespace)
	}
}

// TestSetConfigNamespaceIndependence tests that configuration persists across namespace changes
func TestSetConfigNamespaceIndependence(t *testing.T) {
	// Create kubeconfig with default namespace
	kubeconfigPath := testutils.CreateTestKubeconfig(t, "default")

	params := &common.ResultsParams{}
	cmd := Command(params)

	// Set initial configuration with default namespace
	args := []string{
		"--kubeconfig=" + kubeconfigPath,
		"--context=test-context",
		"set",
		"--host=https://namespace-test.com",
		"--token=namespace-token",
		"--api-path=/api/namespace/test",
	}
	_, err := testutils.ExecuteCommand(cmd, args...)
	if err != nil {
		t.Fatalf("Failed to set initial config: %v", err)
	}

	// Verify configuration was set
	var initialExtension config.Extension
	found, err := testutils.ReadKubeconfigExtension(t, kubeconfigPath, config.ExtensionName, &initialExtension)
	if err != nil {
		t.Fatalf("Failed to read initial extension: %v", err)
	}
	if !found {
		t.Fatalf("Initial configuration not found")
	}
	if initialExtension.Host != "https://namespace-test.com" {
		t.Fatalf("Initial configuration not set correctly, expected host 'https://namespace-test.com', got '%s'", initialExtension.Host)
	}

	// Now simulate switching to a different namespace by updating the kubeconfig
	// This directly tests namespace independence in the same kubeconfig file
	testutils.UpdateKubeconfigNamespace(t, kubeconfigPath, "production")

	// Verify that we can still read the configuration after namespace switch
	// This proves the configuration is namespace-independent
	var extensionAfterNsSwitch config.Extension
	foundAfterSwitch, err := testutils.ReadKubeconfigExtension(t, kubeconfigPath, config.ExtensionName, &extensionAfterNsSwitch)
	if err != nil {
		t.Fatalf("Failed to read extension after namespace switch: %v", err)
	}
	if !foundAfterSwitch {
		t.Fatalf("Configuration not found after namespace switch - this indicates the config is tied to namespace")
	}

	// Verify the values are preserved exactly - proving namespace independence
	if extensionAfterNsSwitch.Host != "https://namespace-test.com" {
		t.Errorf("Configuration not preserved across namespace change: expected host 'https://namespace-test.com', got '%s'", extensionAfterNsSwitch.Host)
	}
	if extensionAfterNsSwitch.Token != "namespace-token" {
		t.Errorf("Configuration not preserved across namespace change: expected token 'namespace-token', got '%s'", extensionAfterNsSwitch.Token)
	}
	if extensionAfterNsSwitch.APIPath != "/api/namespace/test" {
		t.Errorf("Configuration not preserved across namespace change: expected api-path '/api/namespace/test', got '%s'", extensionAfterNsSwitch.APIPath)
	}
}

// TestConfigContextCreation tests that config contexts are created correctly
func TestConfigContextCreation(t *testing.T) {
	kubeconfigPath := testutils.CreateTestKubeconfig(t, "staging")

	params := &common.ResultsParams{}
	cmd := Command(params)

	// Set configuration which should create config context
	args := []string{
		"--kubeconfig=" + kubeconfigPath,
		"--context=test-context",
		"set",
		"--host=https://context-creation-test.com",
		"--token=context-creation-token",
	}
	_, err := testutils.ExecuteCommand(cmd, args...)
	if err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Load kubeconfig and verify config context was created
	configLoadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	apiConfig, err := configLoadingRules.Load()
	if err != nil {
		t.Fatalf("Failed to load kubeconfig: %v", err)
	}

	// Verify config context exists with correct name format
	configContextName := "tekton-results-config/test-cluster/test-user"
	configContext, exists := apiConfig.Contexts[configContextName]
	if !exists {
		t.Fatalf("Config context should be created with name: %s", configContextName)
	}

	// Verify config context has correct properties
	if configContext.Cluster != "test-cluster" {
		t.Errorf("Expected cluster 'test-cluster', got '%s'", configContext.Cluster)
	}
	if configContext.AuthInfo != "test-user" {
		t.Errorf("Expected user 'test-user', got '%s'", configContext.AuthInfo)
	}
	if configContext.Namespace != "default" {
		t.Errorf("Expected namespace 'default', got '%s'", configContext.Namespace)
	}

	// Verify extensions are initialized
	if configContext.Extensions == nil {
		t.Error("Config context extensions should be initialized")
	}
	if configContext.Extensions["tekton-results"] == nil {
		t.Error("Tekton Results extension should exist in config context")
	}
}
