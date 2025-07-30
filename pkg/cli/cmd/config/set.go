package config

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tektoncd/results/pkg/cli/common"
	"github.com/tektoncd/results/pkg/cli/config"
	"github.com/tektoncd/results/pkg/cli/flags"
)

// SetOptions holds options for the set command
type SetOptions struct {
	Config           config.Config
	ResultsNamespace string
}

// setCommand creates a new cobra command for setting the Tekton Results configuration.
//
// Parameters:
//   - p: common.Params containing common parameters for the CLI.
//
// Returns:
//   - *cobra.Command: A pointer to the created cobra command for setting the configuration.
func setCommand(p common.Params) *cobra.Command {
	opts := &SetOptions{}

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set Tekton Results CLI configuration values",
		Long: `Configure settings for the Tekton Results CLI.

This command allows you to configure how the CLI interacts with the Tekton Results API server.
It can automatically detect the API server in your cluster or allow manual configuration.

The command will:
1. Automatically detect platform type (OpenShift vs Kubernetes)
2. Search for routes (OpenShift) or ingresses (Kubernetes) in the appropriate namespace
3. Prompt for any missing configuration values
4. Save the configuration for future use

Detection Strategy:
- OpenShift: Automatically detects routes in openshift-pipelines namespace (default) or custom namespace
- Kubernetes: Automatically detects ingresses in tekton-pipelines namespace (default) or custom namespace

Examples:
  # Automatic detection with default namespace
  tkn-results config set

  # Automatic detection with custom Results namespace
  tkn-results config set --results-namespace=my-tekton-namespace

  # Manual configuration (when automatic detection fails)
  tkn-results config set --host=<api-server-url> --token=<token> --api-path=<path>

  # Manual configuration with custom settings
  tkn-results config set --host=<api-server-url> --token=<token> --api-path=/api/v1 --insecure-skip-tls-verify

Automatic Detection:
- OpenShift: Detects routes in openshift-pipelines namespace (default) or user-provided namespace
- Kubernetes: Detects ingresses in tekton-pipelines namespace (default) or user-provided namespace
- Constructs API URLs from route/ingress configuration
- Filters routes/ingresses by service name (tekton-results-api-service) for better accuracy

Manual Configuration (when automatic detection fails):
- API server host URL
- Authentication token
- API path prefix
- TLS verification settings

If automatic detection fails or RBAC permissions are insufficient, you can provide values manually using the available flags.

Results Namespace:
The --results-namespace flag allows you to specify where Tekton Results is installed:
- Default: tekton-pipelines (Kubernetes) or openshift-pipelines (OpenShift)
- Custom: Use --results-namespace to specify a different namespace

Permission Requirements:
- Namespace access (get namespaces)
- Route access (OpenShift: get routes)
- Ingress access (Kubernetes: get ingresses)
If you encounter permission errors, ask your admin to setup RBAC or use manual configuration.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var err error

			// Initialize the config
			opts.Config, err = config.NewConfig(p)
			if err != nil {
				return err
			}

			// Check if any flags were provided, don't prompt if provided
			changed := flags.AnyResultsFlagChanged(cmd)

			// Validate that results-namespace is not used with manual server configuration
			if opts.ResultsNamespace != "" && changed {
				return fmt.Errorf("--results-namespace flag should not be used when providing manual server configuration (host, token, api-path, etc.). The results-namespace flag is only for auto-detection scenarios")
			}

			return opts.Config.Set(!changed, p, opts.ResultsNamespace)
		},
	}

	// Add global results options
	flags.AddResultsOptions(cmd)

	// Add the results-namespace flag specifically to this command
	cmd.Flags().StringVar(&opts.ResultsNamespace, "results-namespace", "",
		"namespace where Tekton Results is installed (default: tekton-pipelines for K8s, openshift-pipelines for OpenShift)")

	return cmd
}
