package records

import (
	"github.com/spf13/cobra"
	"github.com/tektoncd/results/tools/tkn-results/internal/flags"
)

func Command(params *flags.Params) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "records",
		Short: "Command sub-group for querying Records",
		Annotations: map[string]string{
			"commandType": "main",
		},
	}

	cmd.AddCommand(ListCommand(params), GetRecordCommand(params))

	return cmd
}
