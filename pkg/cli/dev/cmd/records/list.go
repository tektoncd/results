package records

import (
	"fmt"
	"os"

	"github.com/tektoncd/results/pkg/cli/dev/flags"
	"github.com/tektoncd/results/pkg/cli/dev/format"

	"github.com/spf13/cobra"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

// ListRecordsCommand returns a cobra sub command that fetch a list of records given the parent and result name
func ListRecordsCommand(params *flags.Params) *cobra.Command {
	opts := &flags.ListOptions{}

	cmd := &cobra.Command{
		Use:   "list [flags] <result-name>",
		Short: "List Records for a given Result",
		Long:  "List Records for a given Result. <result-name> is typically of format <namespace>/results/<parent-run-uuid>. '-' may be used in place of  <parent-run-uuid> to query all Records for a given parent.",
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := params.ResultsClient.ListRecords(cmd.Context(), &pb.ListRecordsRequest{
				Parent:    args[0],
				Filter:    opts.Filter,
				PageSize:  opts.Limit,
				PageToken: opts.PageToken,
			})
			if err != nil {
				fmt.Printf("ListRecords: %v\n", err)
				return err
			}
			return format.PrintProto(os.Stdout, resp, opts.Format)
		},
		Args: cobra.ExactArgs(1),
		Annotations: map[string]string{
			"commandType": "main",
		},
		Example: `  - List all Records for PipelineRun with UUID 0dfc883d-722a-4489-9ab8-3cccc74ca4f6 in 'default' namespace:
    tkn-results records list default/results/0dfc883d-722a-4489-9ab8-3cccc74ca4f6

  - List all Records for all Runs in 'default' namespace:
    tkn-results records list default/results/-
	
  - List only TaskRuns Records in 'default' namespace:
    tkn-results records list default/results/- --filter="data_type=='tekton.dev/v1beta1.TaskRun'"`,
	}

	flags.AddListFlags(opts, cmd)

	return cmd
}
