package config

import (
	"testing"

	"github.com/tektoncd/results/pkg/cli/common"
	"github.com/tektoncd/results/pkg/cli/config/test"
)

// TestSetCommand tests the setCommand function
func TestSetCommand(t *testing.T) {
	tests := []struct {
		name         string
		params       common.Params
		wantUse      string
		wantShort    string
		wantNoPrompt bool
		wantErr      bool
	}{
		{
			name:         "valid params",
			params:       &test.Params{},
			wantUse:      "set",
			wantShort:    "Configure Tekton Results CLI settings",
			wantNoPrompt: false,
			wantErr:      false,
		},
		{
			name:         "nil params",
			params:       nil,
			wantUse:      "set",
			wantShort:    "Configure Tekton Results CLI settings",
			wantNoPrompt: false,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create the command
			cmd := setCommand(tt.params)
			if cmd == nil {
				t.Fatal("Expected command to be created")
			}

			// Verify command properties
			if cmd.Use != tt.wantUse {
				t.Errorf("Expected Use to be %q, got %q", tt.wantUse, cmd.Use)
			}
			if cmd.Short != tt.wantShort {
				t.Errorf("Expected Short to be %q, got %q", tt.wantShort, cmd.Short)
			}

			// Verify flag exists and has correct default value
			noPromptFlag := cmd.Flag("no-prompt")
			if noPromptFlag == nil {
				t.Fatal("Expected no-prompt flag to exist")
			}

			// Check default value
			if noPromptFlag.Value.String() != "false" {
				t.Errorf("Expected no-prompt flag to be false by default, got %s", noPromptFlag.Value.String())
			}

			// Test flag setting
			if err := noPromptFlag.Value.Set("true"); err != nil {
				t.Errorf("Failed to set no-prompt flag: %v", err)
			}
			if noPromptFlag.Value.String() != "true" {
				t.Errorf("Expected no-prompt flag to be true after setting, got %s", noPromptFlag.Value.String())
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
