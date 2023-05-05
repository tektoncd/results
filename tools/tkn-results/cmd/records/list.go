package records

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"github.com/tektoncd/results/tools/tkn-results/internal/flags"
	"github.com/tektoncd/results/tools/tkn-results/internal/format"
)

func ListCommand(params *flags.Params) *cobra.Command {
	opts := &flags.ListOptions{}

	cmd := &cobra.Command{
		Use: `list [flags] <result parent>

  <result parent>: Result parent name to query. This is typically "<namespace>/results/<result name>", but may vary depending on the API Server. "-" may be used as <result name> to query all Results for a given parent.`,
		Short: "List Records",
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
	}

	flags.AddListFlags(opts, cmd)

	return cmd
}
