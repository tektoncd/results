package records

import (
	"github.com/spf13/cobra"
	"github.com/tektoncd/results/pkg/cli/dev/flags"
)

// Command returns a cobra command for `records` sub commands
func Command(params *flags.Params) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "records",
		Short: "[To be deprecated] Command sub-group for querying Records",
		Annotations: map[string]string{
			"commandType": "main",
		},
	}

	cmd.AddCommand(ListRecordsCommand(params), GetRecordCommand(params))

	return cmd
}
