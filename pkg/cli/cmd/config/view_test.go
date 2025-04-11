package config

import (
	"testing"

	"github.com/tektoncd/results/pkg/cli/common"
	"github.com/tektoncd/results/pkg/cli/config/test"
)

// TestViewCommand tests the viewCommand function
func TestViewCommand(t *testing.T) {
	tests := []struct {
		name      string
		params    common.Params
		wantUse   string
		wantShort string
		wantErr   bool
	}{
		{
			name:      "valid params",
			params:    &test.Params{},
			wantUse:   "view",
			wantShort: "Display current CLI configuration",
			wantErr:   false,
		},
		{
			name:      "nil params",
			params:    nil,
			wantUse:   "view",
			wantShort: "Display current CLI configuration",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create the command
			cmd := viewCommand(tt.params)
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
