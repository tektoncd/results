package config

import (
	"github.com/spf13/cobra"
	"github.com/tektoncd/results/pkg/cli/common"
	"github.com/tektoncd/results/pkg/cli/config"
	"github.com/tektoncd/results/pkg/cli/flags"
)

// SetOptions holds options for the set command
type SetOptions struct {
	Config config.Config
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
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Configure Tekton Results CLI settings",
		Long: `Configure settings for the Tekton Results CLI.

This command allows you to configure how the CLI interacts with the Tekton Results API server.
It can automatically detect the API server in OpenShift environments or allow manual configuration.

The command will:
1. Automatically detect the Tekton Results API server in OpenShift environments
2. Prompt for any missing configuration values
3. Save the configuration for future use

Detection Strategy:
- OpenShift: Automatically detects routes in openshift-pipelines and tekton-results namespaces
- Kubernetes: Manual configuration required (automatic detection not available)

Examples:
  # OpenShift: Automatic detection
  tkn-results config set

  # Kubernetes: Manual configuration (required)
  tkn-results config set --host=<api-server-url> --token=<token> --api-path=<path>

  # Manual configuration with custom settings
  tkn-results config set --host=<api-server-url> --token=<token> --api-path=/api/v1 --insecure-skip-tls-verify

Automatic Detection (OpenShift only):
- Detects routes in openshift-pipelines and tekton-results namespaces
- Constructs API URLs from route configuration
- Uses service account token (if available)
- Filters routes by service name for better accuracy

Manual Configuration (Kubernetes or custom):
- API server host URL
- Authentication token
- API path prefix
- TLS verification settings

If automatic detection fails in OpenShift, you can provide values manually using the available flags.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var err error
			opts.Config, err = config.NewConfig(p)
			if err != nil {
				return err
			}

			// Check if any flags were provided, don't prompt if provided
			changed := flags.AnyResultsFlagChanged(cmd)

			return opts.Config.Set(!changed, p)
		},
	}

	flags.AddResultsOptions(cmd)
	return cmd
}
