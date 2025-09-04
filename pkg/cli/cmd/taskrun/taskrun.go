package taskrun

import (
	"github.com/spf13/cobra"
	"github.com/tektoncd/results/pkg/cli/common"
	"github.com/tektoncd/results/pkg/cli/common/prerun"
	"github.com/tektoncd/results/pkg/cli/flags"
)

// Command returns a cobra command for `tkn-results taskrun` sub commands
func Command(p common.Params) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "taskrun",
		Aliases: []string{"tr"},
		Short:   "Query TaskRuns",
		Long: `Query TaskRuns stored in Tekton Results.

This command allows you to list TaskRuns stored in Tekton Results.
You can filter results by namespace, labels and other criteria.

Examples:
  # List TaskRuns in a namespace
  tkn-results taskrun list -n default

  # List TaskRuns with a specific label
  tkn-results taskrun list -L app=myapp

  # List TaskRuns from all namespaces
  tkn-results taskrun list -A

  # List TaskRuns with limit
  tkn-results taskrun list --limit=20`,
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

	// Add flags to the taskrun command
	flags.AddResultsOptions(cmd)

	cmd.AddCommand(listCommand(p))
	cmd.AddCommand(logsCommand(p))
	cmd.AddCommand(describeCommand(p))

	return cmd
}
