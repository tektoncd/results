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
		Short: "Configure Tekton Results CLI settings",
		Long: `Configure settings for the Tekton Results CLI.

This command allows you to configure how the CLI interacts with the Tekton Results API server.
It can automatically detect the API server in your cluster or allow manual configuration.

The command will:
1. Automatically detect the Tekton Results API server in your cluster
2. Prompt for any missing configuration values
3. Save the configuration for future use

Automatic Detection:
- Cluster context and namespace
- API server endpoint
- Service account token (if available)

Manual Configuration (if automatic detection fails):
- API server host (e.g., http://localhost:8080)
- Authentication token
- Additional cluster settings

Configuration Options:
  --host                    API server host URL
  --token                   Authentication token
  --kubeconfig, -k          Path to kubeconfig file
  --context, -c             Kubernetes context to use
  --namespace, -n           Kubernetes namespace
  --api-path                API server path prefix
  --insecure-skip-tls-verify Skip TLS certificate verification

Note: When using configuration flags, you must also use --no-prompt to skip interactive prompts.

Examples:
  # Configure with automatic detection and interactive prompts
  tkn-results config set

  # Configure with specific parameters (must use --no-prompt)
  tkn-results config set --no-prompt --host=http://localhost:8080 --token=my-token

  # Configure with custom kubeconfig and context (must use --no-prompt)
  tkn-results config set --no-prompt --kubeconfig=/path/to/kubeconfig --context=my-cluster`,
		RunE: func(_ *cobra.Command, _ []string) error {
			var err error
			opts.Config, err = config.NewConfig(p)
			if err != nil {
				return err
			}
			return opts.Config.Set(!opts.NoPrompt, p)
		},
	}
	c.Flags().BoolVarP(&opts.NoPrompt, "no-prompt", "", opts.NoPrompt, "Skip interactive prompts and use default values")
	return c
}
