package pipelinerun

import (
	"testing"

	"github.com/tektoncd/results/pkg/cli/testutils"
)

func TestCommand(t *testing.T) {
	// Create test params
	params := testutils.NewParams()
	params.SetHost("http://localhost:8080")

	// Get the command
	cmd := Command(params)

	// Test command configuration
	t.Run("command configuration", func(t *testing.T) {
		// Check command name and aliases
		if cmd.Use != "pipelinerun" {
			t.Errorf("unexpected command name: got %v, want %v", cmd.Use, "pipelinerun")
		}
		if len(cmd.Aliases) != 2 || cmd.Aliases[0] != "pr" {
			t.Errorf("unexpected aliases: got %v, want [pr]", cmd.Aliases)
		}

		// Check command descriptions
		if cmd.Short != "Query PipelineRuns" {
			t.Errorf("unexpected short description: got %v, want 'Query PipelineRuns'", cmd.Short)
		}
		if cmd.PersistentPreRunE == nil {
			t.Error("command should have PersistentPreRunE")
		}

		// Check command type annotation
		if cmdType, ok := cmd.Annotations["commandType"]; !ok || cmdType != "main" {
			t.Errorf("unexpected command type annotation: got %v, want 'main'", cmdType)
		}
	})

	t.Run("subcommands", func(t *testing.T) {
		// Check if all expected subcommands are registered
		expectedSubcommands := []string{"list", "describe", "logs"}

		for _, subcmdName := range expectedSubcommands {
			subcmd, _, err := cmd.Find([]string{subcmdName})
			if err != nil {
				t.Errorf("%s subcommand not found: %v", subcmdName, err)
			}
			if subcmd.Name() != subcmdName {
				t.Errorf("unexpected subcommand name: got %v, want '%s'", subcmd.Name(), subcmdName)
			}
		}
	})
}
