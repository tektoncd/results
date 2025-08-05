package config

import (
	"testing"

	"github.com/tektoncd/results/pkg/cli/common"
)

// TestCommand tests basic command creation and structure
func TestCommand(t *testing.T) {
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
			cmd := Command(tt.params)
			if cmd == nil {
				t.Fatal("expected command to be created")
			}

			// Verify basic command properties
			if cmd.Use != "config" {
				t.Errorf("expected Use to be %q, got %q", "config", cmd.Use)
			}
			if cmd.Short != "Manage Tekton Results CLI configuration" {
				t.Errorf("expected Short to be %q, got %q", "Manage Tekton Results CLI configuration", cmd.Short)
			}

			// Verify specific subcommands exist
			subcmds := cmd.Commands()
			expectedSubcmds := []string{"set", "reset", "view"}
			if len(subcmds) != len(expectedSubcmds) {
				t.Errorf("expected %d subcommands, got %d", len(expectedSubcmds), len(subcmds))
			}

			for _, expectedCmd := range expectedSubcmds {
				found := false
				for _, subcmd := range subcmds {
					if subcmd.Use == expectedCmd {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected subcommand %q to exist", expectedCmd)
				}
			}

			// Verify PersistentPreRunE is set
			if cmd.PersistentPreRunE == nil {
				t.Error("expected PersistentPreRunE to be set")
			}
		})
	}
}

// TestCommandPersistentPreRunE tests the persistent pre-run function
func TestCommandPersistentPreRunE(t *testing.T) {
	params := &common.ResultsParams{}
	cmd := Command(params)

	if cmd.PersistentPreRunE == nil {
		t.Fatal("PersistentPreRunE should not be nil")
	}

	// Parse flags first to set up the command properly
	cmd.SetArgs([]string{})
	err := cmd.ParseFlags([]string{})
	if err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	// Test that it doesn't error with valid setup
	err = cmd.PersistentPreRunE(cmd, []string{})
	if err != nil {
		t.Errorf("expected no error for valid params, got: %v", err)
	}
}
