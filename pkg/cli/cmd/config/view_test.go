package config

import (
	"strings"
	"testing"

	"github.com/tektoncd/results/pkg/cli/common"
	configpkg "github.com/tektoncd/results/pkg/cli/config"
	"github.com/tektoncd/results/pkg/cli/testutils"
	testutil "github.com/tektoncd/results/pkg/test"
)

// TestViewCommand tests basic view command creation
func TestViewCommand(t *testing.T) {
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
			cmd := viewCommand(tt.params)

			if cmd == nil {
				t.Error("expected command to be created")
				return
			}

			if cmd.Use != "view" {
				t.Errorf("expected command use to be 'view', got %q", cmd.Use)
			}
		})
	}
}

// TestViewCommandExecution tests actual view command execution
func TestViewCommandExecution(t *testing.T) {
	tests := []struct {
		name          string
		setupConfig   bool
		expectHost    string
		expectToken   string
		expectAPIPath string
		description   string
	}{
		{
			name:          "view existing configuration",
			setupConfig:   true,
			expectHost:    "https://test-view.com",
			expectToken:   "view-token",
			expectAPIPath: "/api/v1",
			description:   "should display existing configuration values",
		},
		{
			name:          "view empty configuration",
			setupConfig:   false,
			expectHost:    "",
			expectToken:   "",
			expectAPIPath: "",
			description:   "should display empty values when no configuration exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeconfigPath := testutils.CreateTestKubeconfig(t)

			// Create test params
			params := &common.ResultsParams{}
			params.SetKubeConfigPath(kubeconfigPath)
			params.SetKubeContext("test-context")

			if tt.setupConfig {
				// First, set up a configuration to view
				cmd := Command(params)
				setArgs := []string{"--kubeconfig=" + kubeconfigPath, "set", "--host=https://test-view.com", "--token=view-token", "--api-path=/api/v1"}
				_, err := testutil.ExecuteCommand(cmd, setArgs...)
				if err != nil {
					t.Fatalf("failed to set initial config: %v", err)
				}

				// Verify configuration was actually set by reading it back
				var extension configpkg.Extension
				found, err := testutils.ReadKubeconfigExtension(t, kubeconfigPath, configpkg.ExtensionName, &extension)
				if err != nil {
					t.Fatalf("failed to read config after set: %v", err)
				}
				if !found || extension.Host != "https://test-view.com" {
					t.Fatal("configuration was not set properly for view test")
				}
			}

			// Now test view command and capture output
			cmd := Command(params)
			viewArgs := []string{"--kubeconfig=" + kubeconfigPath, "view"}
			output, err := testutil.ExecuteCommand(cmd, viewArgs...)
			if err != nil {
				t.Errorf("%s: unexpected error: %v", tt.description, err)
				return
			}

			// Verify output contains expected values
			t.Logf("View command output:\n%s", output)

			// Check for expected host
			if tt.expectHost != "" {
				expectedHostLine := "host: " + tt.expectHost
				if !strings.Contains(output, expectedHostLine) {
					t.Errorf("%s: expected output to contain %q, but it was missing.\nFull output:\n%s",
						tt.description, expectedHostLine, output)
				}
			} else if !strings.Contains(output, `host: ""`) {
				t.Errorf("%s: expected output to contain empty host, but it was missing.\nFull output:\n%s",
					tt.description, output)
			}

			// Check for expected token
			if tt.expectToken != "" {
				expectedTokenLine := "token: " + tt.expectToken
				if !strings.Contains(output, expectedTokenLine) {
					t.Errorf("%s: expected output to contain %q, but it was missing.\nFull output:\n%s",
						tt.description, expectedTokenLine, output)
				}
			} else if !strings.Contains(output, `token: ""`) {
				t.Errorf("%s: expected output to contain empty token, but it was missing.\nFull output:\n%s",
					tt.description, output)
			}

			// Check for expected API path (only if specified)
			if tt.expectAPIPath != "" {
				expectedAPIPathLine := "api-path: " + tt.expectAPIPath
				if !strings.Contains(output, expectedAPIPathLine) {
					t.Errorf("%s: expected output to contain %q, but it was missing.\nFull output:\n%s",
						tt.description, expectedAPIPathLine, output)
				}
			}

			// Verify output contains YAML structure
			if !strings.Contains(output, "apiVersion: results.tekton.dev/v1alpha2") {
				t.Errorf("%s: expected output to contain apiVersion, but it was missing.\nFull output:\n%s",
					tt.description, output)
			}
			if !strings.Contains(output, "kind: Client") {
				t.Errorf("%s: expected output to contain kind, but it was missing.\nFull output:\n%s",
					tt.description, output)
			}

			// Verify output is not empty
			if len(output) == 0 {
				t.Errorf("%s: expected some output, but got empty string", tt.description)
			}
		})
	}
}
