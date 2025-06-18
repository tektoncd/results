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
		Use:   "config",
		Short: "Manage Tekton Results CLI configuration",
		Long: `Manage configuration settings for the Tekton Results CLI.

This command allows you to configure how the CLI interacts with the Tekton Results API server.
You can set, view, and reset configuration values such as:
- API server endpoint
- Authentication token
- Cluster context and namespace
- TLS verification settings

Available subcommands:
  set    - Configure CLI settings
  view   - Display current configuration
  reset  - Reset configuration to defaults

Examples:
  # View current configuration
  tkn-results config view

  # Configure with automatic detection
  tkn-results config set

  # Configure with specific parameters
  tkn-results config set --host=http://localhost:8080 --token=my-token

  # Reset configuration to defaults
  tkn-results config reset`,
		PersistentPreRunE: prerun.PersistentPreRunE(p),
		Annotations: map[string]string{
			"commandType": "main",
		},
	}

	// Add flags to the config command
	flags.AddResultsOptions(cmd)

	cmd.AddCommand(
		setCommand(p),
		resetCommand(p),
		viewCommand(p),
	)

	return cmd
}
