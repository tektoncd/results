package pipelinerun

import (
	"github.com/spf13/cobra"
	"github.com/tektoncd/results/pkg/cli/common"
	"github.com/tektoncd/results/pkg/cli/common/prerun"
	"github.com/tektoncd/results/pkg/cli/flags"
)

// Command returns a cobra command for `tkn-results pipelinerun` sub commands
func Command(p common.Params) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pipelinerun",
		Aliases: []string{"pr", "pipelineruns"},
		Short:   "Query PipelineRuns",
		Long: `Query PipelineRuns stored in Tekton Results.

This command allows you to list PipelineRuns stored in Tekton Results.
You can filter results by namespace, labels and other criteria.

Examples:
  # List PipelineRuns in a namespace
  tkn-results pipelinerun list -n default

  # List PipelineRuns with a specific label
  tkn-results pipelinerun list -L app=myapp

  # List PipelineRuns from all namespaces
  tkn-results pipelinerun list -A

  # List PipelineRuns with limit
  tkn-results pipelinerun list --limit=20`,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// Initialize params from flags first
			if err := flags.InitParams(p, cmd); err != nil {
				return err
			}
			if p.RESTClient() == nil {
				restClient, err := prerun.InitClient(p, cmd)
				if err != nil {
					return err
				}
				p.SetRESTClient(restClient)
			}
			return nil
		},
		Annotations: map[string]string{
			"commandType": "main",
		},
	}

	// Add flags to the pipelinerun command
	flags.AddResultsOptions(cmd)

	cmd.AddCommand(listCommand(p))
	cmd.AddCommand(logsCommand(p))
	cmd.AddCommand(describeCommand(p))

	return cmd
}
