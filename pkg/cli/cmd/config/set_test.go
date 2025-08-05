package config

import (
	"strings"
	"testing"

	"github.com/tektoncd/results/pkg/cli/common"
	"github.com/tektoncd/results/pkg/cli/config"
	"github.com/tektoncd/results/pkg/cli/testutils"
	testutil "github.com/tektoncd/results/pkg/test"
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
			name:         "results-namespace only should prompt for host",
			args:         []string{"set", "--results-namespace=test-ns"},
			stdinInput:   "", // Empty input should cause error
			expectPrompt: true,
			expectError:  true,
			description:  "results-namespace alone should still trigger prompting for other values",
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
			kubeconfigPath, cleanup := testutils.CreateTestKubeconfigForExecution(t)
			defer cleanup()

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
					t.Errorf("%s: Expected error but got none", tt.description)
				}
				return
			}

			// Should not have error for success cases
			if err != nil {
				t.Errorf("%s: Unexpected error: %v", tt.description, err)
				return
			}

			// Check prompt expectations by examining output
			output := stdout.String() + stderr.String()
			containsPrompt := strings.Contains(output, "Host:") || strings.Contains(output, "Token:") ||
				strings.Contains(output, "Please select") || strings.Contains(output, "Use the arrow keys")

			if tt.expectPrompt && !containsPrompt {
				t.Errorf("%s: Expected prompting but found no prompt messages in output", tt.description)
			}

			if !tt.expectPrompt && containsPrompt {
				t.Errorf("%s: Expected no prompting but found prompt messages in output", tt.description)
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
		{
			name:        "set with custom results-namespace and manual config should fail",
			args:        []string{"--results-namespace=custom-ns", "--host=https://custom-host.com", "--token=custom-token"},
			expectError: true,
			description: "should fail when results-namespace is used with manual configuration",
		},
		{
			name:        "set with only results-namespace should prompt and fail",
			args:        []string{"--results-namespace=nonexistent-ns"},
			expectError: true,
			description: "should attempt host detection and fail appropriately",
		},
		{
			name:        "results-namespace with host should fail",
			args:        []string{"--results-namespace=test-namespace", "--host=https://namespace-test.com"},
			expectError: true,
			description: "should fail when results-namespace is used with manual host configuration",
		},
		{
			name:        "results-namespace with token should fail",
			args:        []string{"--results-namespace=test-namespace", "--token=test-token"},
			expectError: true,
			description: "should fail when results-namespace is used with manual token configuration",
		},
		{
			name:        "results-namespace with api-path should fail",
			args:        []string{"--results-namespace=test-namespace", "--api-path=/api/v1"},
			expectError: true,
			description: "should fail when results-namespace is used with manual api-path configuration",
		},
		{
			name:        "results-namespace with multiple manual flags should fail",
			args:        []string{"--results-namespace=test-namespace", "--host=https://test.com", "--token=token"},
			expectError: true,
			description: "should fail when results-namespace is used with multiple manual configuration flags",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary kubeconfig
			kubeconfigPath, cleanup := testutils.CreateTestKubeconfigForExecution(t)
			defer cleanup()

			// Create test params
			params := &common.ResultsParams{}
			params.SetKubeConfigPath(kubeconfigPath)
			params.SetKubeContext("test-context")

			// Get command
			cmd := Command(params)

			// Add kubeconfig to args and execute
			allArgs := append([]string{"--kubeconfig=" + kubeconfigPath, "set"}, tt.args...)
			_, err := testutil.ExecuteCommand(cmd, allArgs...)

			// Check error expectations
			if tt.expectError {
				if err == nil {
					t.Errorf("%s: Expected error but got none", tt.description)
				}
				return
			}

			// Should not have error for success cases
			if err != nil {
				t.Errorf("%s: Unexpected error: %v", tt.description, err)
				return
			}

			// Verify the configuration was persisted
			var extension config.Extension
			found, err := testutils.ReadKubeconfigExtension(t, kubeconfigPath, config.ExtensionName, &extension)
			if err != nil {
				t.Fatalf("Failed to read persisted config: %v", err)
			}
			if !found {
				t.Fatalf("Expected configuration to be persisted, but extension not found")
			}

			// Basic verification that values were set
			if extension.Host == "" {
				t.Errorf("Expected host to be set in persisted config")
			}
			if extension.Token == "" {
				t.Errorf("Expected token to be set in persisted config")
			}
		})
	}
}

func TestSetCommandConfigOverwrite(t *testing.T) {
	// Create temporary kubeconfig
	kubeconfigPath, cleanup := testutils.CreateTestKubeconfigForExecution(t)
	defer cleanup()

	// Create command
	params := &common.ResultsParams{}
	params.SetKubeConfigPath(kubeconfigPath)
	params.SetKubeContext("test-context")
	cmd := Command(params)

	// First, set initial configuration
	args1 := []string{"--kubeconfig=" + kubeconfigPath, "set", "--host=https://first-host.com", "--token=first-token"}
	_, err := testutil.ExecuteCommand(cmd, args1...)
	if err != nil {
		t.Fatalf("Failed to set initial config: %v", err)
	}

	// Verify first configuration was set
	var extension1 config.Extension
	found1, err := testutils.ReadKubeconfigExtension(t, kubeconfigPath, config.ExtensionName, &extension1)
	if err != nil {
		t.Fatalf("Failed to read first extension: %v", err)
	}
	if !found1 || extension1.Host != "https://first-host.com" {
		t.Fatalf("First configuration not set correctly")
	}

	// Now overwrite with new configuration
	args2 := []string{"--kubeconfig=" + kubeconfigPath, "set", "--host=https://second-host.com", "--token=second-token", "--api-path=/v2"}
	_, err = testutil.ExecuteCommand(cmd, args2...)
	if err != nil {
		t.Fatalf("Failed to overwrite config: %v", err)
	}

	// Verify second configuration overwrote the first
	var extension2 config.Extension
	found2, err := testutils.ReadKubeconfigExtension(t, kubeconfigPath, config.ExtensionName, &extension2)
	if err != nil {
		t.Fatalf("Failed to read second config: %v", err)
	}
	if !found2 {
		t.Fatalf("Expected second configuration to be persisted, but extension not found")
	}

	if extension2.Host != "https://second-host.com" {
		t.Errorf("Expected host %q, got %q", "https://second-host.com", extension2.Host)
	}
	if extension2.Token != "second-token" {
		t.Errorf("Expected token %q, got %q", "second-token", extension2.Token)
	}
	if extension2.APIPath != "/v2" {
		t.Errorf("Expected api-path %q, got %q", "/v2", extension2.APIPath)
	}
}
