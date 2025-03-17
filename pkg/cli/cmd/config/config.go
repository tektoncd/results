package config

import (
	"github.com/spf13/cobra"
	"github.com/tektoncd/results/pkg/cli/common"
	"github.com/tektoncd/results/pkg/cli/common/prerun"
	"github.com/tektoncd/results/pkg/cli/flags"
)

// Command creates and returns a new cobra.Command for the 'config' subcommand.
// It sets up the command structure for managing Results configuration,
// including options to set, view, and reset the config.
//
// Parameters:
//   - p: common.Params - A struct containing common parameters used across the CLI.
//
// Returns:
//   - *cobra.Command: A pointer to the newly created cobra.Command for the 'config' subcommand.
func Command(p common.Params) *cobra.Command {
    cmd := &cobra.Command{
        Use:               "config",
        Short:             "set, reset or view results config",
        PersistentPreRunE: prerun.PersistentPreRunE(p),
    }

    flags.AddResultsOptions(cmd)

    cmd.AddCommand(
        setCommand(p),
        viewCommand(p),
        resetCommand(p),
    )

    return cmd
}
