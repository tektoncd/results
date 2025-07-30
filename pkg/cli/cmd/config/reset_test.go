package config

import (
	"testing"

	"github.com/tektoncd/results/pkg/cli/common"
	configpkg "github.com/tektoncd/results/pkg/cli/config"
	"github.com/tektoncd/results/pkg/cli/testutils"
	testutil "github.com/tektoncd/results/pkg/test"
)

// TestResetCommand tests basic reset command creation
func TestResetCommand(t *testing.T) {
	tests := []struct {
		name   string
		params common.Params
	}{
		{
			name:   "valid params",
			params: &common.ResultsParams{},
		},
		{
			name:   "nil params",
			params: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := resetCommand(tt.params)

			if cmd == nil {
				t.Error("Expected command to be created")
				return
			}

			if cmd.Use != "reset" {
				t.Errorf("Expected command use to be 'reset', got %q", cmd.Use)
			}
		})
	}
}

// TestResetCommandExecution tests actual reset command execution
func TestResetCommandExecution(t *testing.T) {
	tests := []struct {
		name                   string
		existingConfig         bool
		expectConfigAfterReset bool
		description            string
	}{
		{
			name:                   "reset existing configuration",
			existingConfig:         true,
			expectConfigAfterReset: false,
			description:            "should reset existing configuration and remove tekton-results extension",
		},
		{
			name:                   "reset non-existent configuration",
			existingConfig:         false,
			expectConfigAfterReset: false,
			description:            "should handle reset gracefully when no configuration exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeconfigPath, cleanup := testutils.CreateTestKubeconfigForExecution(t)
			defer cleanup()

			// Set up params
			params := &common.ResultsParams{}
			params.SetKubeConfigPath(kubeconfigPath)
			params.SetKubeContext("test-context")

			if tt.existingConfig {
				// First, set up a configuration to reset
				cmd := Command(params)
				setArgs := []string{"--kubeconfig=" + kubeconfigPath, "set", "--host=https://test-reset.com", "--token=reset-token"}
				_, err := testutil.ExecuteCommand(cmd, setArgs...)
				if err != nil {
					t.Fatalf("Failed to set up test config: %v", err)
				}

				// Verify configuration was set
				var extension configpkg.Extension
				found, err := testutils.ReadKubeconfigExtension(t, kubeconfigPath, configpkg.ExtensionName, &extension)
				if err != nil {
					t.Fatalf("Failed to verify test config was set: %v", err)
				}
				if !found || extension.Host != "https://test-reset.com" {
					t.Fatalf("Test configuration was not set properly")
				}
			}

			// Now test the reset command
			cmd := Command(params)
			resetArgs := []string{"--kubeconfig=" + kubeconfigPath, "reset"}
			_, err := testutil.ExecuteCommand(cmd, resetArgs...)
			if err != nil {
				t.Errorf("%s: Unexpected error during reset: %v", tt.description, err)
				return
			}

			// Verify configuration was reset (extension should be empty or have empty host)
			var extension configpkg.Extension
			found, err := testutils.ReadKubeconfigExtension(t, kubeconfigPath, configpkg.ExtensionName, &extension)
			if err != nil {
				t.Errorf("%s: Failed to read config after reset: %v", tt.description, err)
				return
			}

			// After reset, extension might not be found or have empty host
			if found && extension.Host != "" {
				t.Errorf("%s: Expected configuration to be reset, but host is still set to: %q", tt.description, extension.Host)
			}
		})
	}
}
