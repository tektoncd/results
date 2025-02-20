package pipelinerun

import (
	"github.com/spf13/cobra"
	"github.com/tektoncd/results/pkg/cli/flags"
)

// Command initializes a cobra command for `pipelinerun` sub commands
func Command(params *flags.Params) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pipelinerun",
		Aliases: []string{"pr", "pipelineruns"},
		Short:   "Query PipelineRuns",
		Annotations: map[string]string{
			"commandType": "main",
		},
	}

	cmd.AddCommand(listCommand(params))

	return cmd
}
