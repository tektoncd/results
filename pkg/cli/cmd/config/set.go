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

// setCommand creates a new cobra command for setting the Tekton Results configuration.
//
// Parameters:
//   - p: common.Params containing common parameters for the CLI.
//
// Returns:
//   - *cobra.Command: A pointer to the created cobra command for setting the configuration.
func setCommand(p common.Params) *cobra.Command {
	opts := &SetOptions{}

	eg := `Configure with automatic detection and interactive prompts:
  tkn-results config set

Configure with specific parameters (no prompts):
  tkn-results config set --host=http://localhost:8080 --token=my-token

Configure with custom API path (no prompts):
  tkn-results config set --api-path=/api/v1

Configure with custom kubeconfig and context:
  tkn-results config set --kubeconfig=/path/to/kubeconfig --context=my-cluster`

	cmd := &cobra.Command{
		Use:     "set",
		Short:   "Set Tekton Results CLI configuration values",
		Example: eg,
		Long: `Configure how the CLI connects to the Tekton Results API server.

Configuration Storage:
The configuration is stored in a namespace-independent way in your kubeconfig file.
This means the configuration persists across namespace switches (e.g., 'kubectl config 
set-context --current --namespace=production' or 'oc project production').
You only need to configure once per cluster/user combination.

Usage Modes:
1. Interactive: Prompts for values with defaults where available
   ` + "`" + `tkn-results config set` + "`" + `

2. Manual: Specify values via flags
   ` + "`" + `tkn-results config set --host=<url> --token=<token>` + "`" + `

Configuration Options:
- Host: Tekton Results API server URL
- Token: Bearer token (defaults to current kubeconfig token)
- API Path: API endpoint path
- TLS Settings: Certificate verification options

Use manual configuration when:
- Route is not in openshift-pipelines namespace
- Route name differs from tekton-results-api-service
- Using custom domain patterns
- On Kubernetes clusters (ingress hostnames vary)

Route Requirements (OpenShift):
- Route name: tekton-results-api-service
- Namespace: openshift-pipelines
- Expected URL format: ` + "`" + `https://<route-name>-<namespace>.apps.<cluster-domain>` + "`" + `

If your route deviates from this standard format, use manual configuration.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var err error

			// Initialize the config
			opts.Config, err = config.NewConfig(p)
			if err != nil {
				return err
			}

			// Check if any flags were provided, don't prompt if provided
			changed := flags.AnyResultsFlagChanged(cmd)

			return opts.Config.Set(!changed, p)
		},
	}

	// Add global results options
	flags.AddResultsOptions(cmd)

	return cmd
}
