package result

import (
	"github.com/spf13/cobra"
	"github.com/tektoncd/results/pkg/cli/flags"
)

// Command initializes a cobra command for `pipelinerun` sub commands
func Command(params *flags.Params) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "result",
		Aliases: []string{"r", "results"},
		Short:   "Query Results",
		Annotations: map[string]string{
			"commandType": "main",
		},
	}

	cmd.AddCommand(
		ListCommand(params),
		describeCommand(params),
	)

	return cmd
}
