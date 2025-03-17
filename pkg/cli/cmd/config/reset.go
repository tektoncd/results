package config

import (
	"github.com/spf13/cobra"
	"github.com/tektoncd/results/pkg/cli/common"
	"github.com/tektoncd/results/pkg/cli/config"
)

// ResetOptions holds the configuration for the reset command.
type ResetOptions struct {
    Config config.Config
}

// resetCommand creates a new cobra command for resetting the Tekton Results configuration.
//
// Parameters:
//   - p: common.Params containing common parameters for the CLI.
//
// Returns:
//   - *cobra.Command: A pointer to the created cobra command for resetting the configuration.
func resetCommand(p common.Params) *cobra.Command {
    opts := &ResetOptions{}
    c := &cobra.Command{
        Use:   "reset",
        Short: "Reset Tekton Results config",
        RunE: func(cmd *cobra.Command, _ []string) error {
            var err error
            opts.Config, err = config.NewConfig(p)
            if err != nil {
                return err
            }
            return opts.Config.Reset(p)
        },
    }
    return c
}
