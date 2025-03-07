package config

import (
	"github.com/spf13/cobra"
	"github.com/tektoncd/results/pkg/cli/common"
	"github.com/tektoncd/results/pkg/cli/config"
)

// SetOptions holds the configuration options for the set command.
type SetOptions struct {
	Config   config.Config
	NoPrompt bool
}

// setCommand creates a new cobra.Command for setting Tekton Results configuration.
// It initializes the configuration, handles user prompts, and applies the settings.
//
// Parameters:
//   - p: common.Params containing shared parameters for CLI commands.
//
// Returns:
//   - *cobra.Command: A cobra.Command object that can be executed to set the configuration.
func setCommand(p common.Params) *cobra.Command {
	opts := &SetOptions{}
	c := &cobra.Command{
		Use:   "set",
		Short: "Set Tekton Results config",
		RunE: func(_ *cobra.Command, _ []string) error {
			var err error
			opts.Config, err = config.NewConfig(p)
			if err != nil {
				return err
			}
			return opts.Config.Set(!opts.NoPrompt, p)
		},
	}
	c.Flags().BoolVarP(&opts.NoPrompt, "no-prompt", "", opts.NoPrompt, "do not prompt for the user input")
	return c
}
