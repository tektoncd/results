package config

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/tektoncd/results/pkg/cli/common"
	"github.com/tektoncd/results/pkg/cli/config/test"
	"github.com/tektoncd/results/pkg/cli/flags"
)

// TestSetCommand tests the setCommand function
func TestSetCommand(t *testing.T) {
	tests := []struct {
		name       string
		params     common.Params
		wantUse    string
		wantShort  string
		flags      map[string]string
		wantPrompt bool
		wantErr    bool
	}{
		{
			name:       "valid params no flags",
			params:     &test.Params{},
			wantUse:    "set",
			wantShort:  "Configure Tekton Results CLI settings",
			flags:      map[string]string{},
			wantPrompt: true,
			wantErr:    false,
		},
		{
			name:       "nil params no flags",
			params:     nil,
			wantUse:    "set",
			wantShort:  "Configure Tekton Results CLI settings",
			flags:      map[string]string{},
			wantPrompt: true,
			wantErr:    false,
		},
		{
			name:       "with host flag",
			params:     &test.Params{},
			wantUse:    "set",
			wantShort:  "Configure Tekton Results CLI settings",
			flags:      map[string]string{"host": "http://example.com"},
			wantPrompt: false,
			wantErr:    false,
		},
		{
			name:       "with token flag",
			params:     &test.Params{},
			wantUse:    "set",
			wantShort:  "Configure Tekton Results CLI settings",
			flags:      map[string]string{"token": "test-token"},
			wantPrompt: false,
			wantErr:    false,
		},
		{
			name:       "with api-path flag",
			params:     &test.Params{},
			wantUse:    "set",
			wantShort:  "Configure Tekton Results CLI settings",
			flags:      map[string]string{"api-path": "/api/v1"},
			wantPrompt: false,
			wantErr:    false,
		},
		{
			name:       "with insecure-skip-tls-verify flag",
			params:     &test.Params{},
			wantUse:    "set",
			wantShort:  "Configure Tekton Results CLI settings",
			flags:      map[string]string{"insecure-skip-tls-verify": "true"},
			wantPrompt: false,
			wantErr:    false,
		},
		{
			name:       "with multiple flags",
			params:     &test.Params{},
			wantUse:    "set",
			wantShort:  "Configure Tekton Results CLI settings",
			flags:      map[string]string{"host": "http://example.com", "token": "test-token"},
			wantPrompt: false,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create the full command hierarchy
			rootCmd := Command(tt.params)

			// Find the set command by name
			var cmd *cobra.Command
			for _, c := range rootCmd.Commands() {
				if c.Use == "set" {
					cmd = c
					break
				}
			}
			if cmd == nil {
				t.Fatal("Could not find 'set' command")
			}

			// Verify command properties
			if cmd.Use != tt.wantUse {
				t.Errorf("Expected Use to be %q, got %q", tt.wantUse, cmd.Use)
			}
			if cmd.Short != tt.wantShort {
				t.Errorf("Expected Short to be %q, got %q", tt.wantShort, cmd.Short)
			}

			// Initialize the flags by parsing empty args
			if err := cmd.ParseFlags([]string{}); err != nil {
				t.Fatalf("Failed to parse flags: %v", err)
			}

			// Set flags if provided
			for flag, value := range tt.flags {
				if err := cmd.Flags().Set(flag, value); err != nil {
					t.Errorf("Failed to set %q flag: %v", flag, err)
				}
			}

			// Verify prompting behavior
			changed := flags.AnyResultsFlagChanged(cmd)
			if !changed != tt.wantPrompt {
				t.Errorf("Expected prompt to be %v, got %v", tt.wantPrompt, !changed)
			}

			// Test command execution
			if tt.wantErr {
				if cmd.RunE == nil {
					t.Error("Expected RunE to be set for error case")
				}
			} else {
				if cmd.RunE == nil {
					t.Error("Expected RunE to be set")
				}
			}
		})
	}
}
