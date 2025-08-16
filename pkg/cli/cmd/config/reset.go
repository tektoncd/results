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

	eg := `Reset all configuration settings:
  tkn-results config reset

Reset and verify the changes:
  tkn-results config reset && tkn-results config view

Reset and immediately reconfigure:
  tkn-results config reset && tkn-results config set`

	c := &cobra.Command{
		Use:     "reset",
		Short:   "Reset CLI configuration to defaults",
		Example: eg,
		Long: `Reset all configuration settings to their default values.

This command will:
1. Remove all custom configuration settings
2. Reset to default values:
   - API server endpoint
   - Authentication token
   - Cluster context and namespace
   - TLS verification settings

Warning: This will remove all custom configuration settings.
         You will need to reconfigure the CLI after resetting.`,
		RunE: func(_ *cobra.Command, _ []string) error {
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
