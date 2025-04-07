package taskrun

import (
	"testing"

	"github.com/tektoncd/results/pkg/cli/common"
)

type testParams struct {
	*common.ResultsParams
}

func (p *testParams) SetHost(host string) {
	p.ResultsParams.SetHost(host)
}

func TestCommand(t *testing.T) {
	// Create test params
	params := &testParams{
		ResultsParams: &common.ResultsParams{},
	}
	params.SetHost("http://localhost:8080")

	// Get the command
	cmd := Command(params)

	// Test command configuration
	t.Run("command configuration", func(t *testing.T) {
		// Check command name and aliases
		if cmd.Use != "taskrun" {
			t.Errorf("unexpected command name: got %v, want %v", cmd.Use, "taskrun")
		}
		if len(cmd.Aliases) != 1 || cmd.Aliases[0] != "tr" {
			t.Errorf("unexpected aliases: got %v, want [tr]", cmd.Aliases)
		}

		// Check command descriptions
		if cmd.Short != "Query TaskRuns" {
			t.Errorf("unexpected short description: got %v, want 'Query TaskRuns'", cmd.Short)
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
		// Check if list subcommand is registered
		listCmd, _, err := cmd.Find([]string{"list"})
		if err != nil {
			t.Errorf("list subcommand not found: %v", err)
		}
		if listCmd.Name() != "list" {
			t.Errorf("unexpected subcommand name: got %v, want 'list'", listCmd.Name())
		}
	})
}
