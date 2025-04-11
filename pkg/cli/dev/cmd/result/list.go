package result

import (
	"fmt"
	"os"

	"github.com/tektoncd/results/pkg/cli/dev/flags"
	"github.com/tektoncd/results/pkg/cli/dev/format"

	"github.com/spf13/cobra"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

// ListCommand returns a cobra sub command that fetch a list of results given the parent name
func ListCommand(params *flags.Params) *cobra.Command {
	opts := &flags.ListOptions{}

	cmd := &cobra.Command{
		Use: `list [flags] <parent>

  <parent>: Parent name to query. This is typically corresponds to a namespace, but may vary depending on the API Server. "-" may be used to query all parents. This will list results for namespaces the token has access to`,
		Short: "List Results",
		RunE: func(cmd *cobra.Command, args []string) error {
			parent := args[0]
			resp, err := params.ResultsClient.ListResults(cmd.Context(), &pb.ListResultsRequest{
				Parent:    parent,
				Filter:    opts.Filter,
				PageSize:  opts.Limit,
				PageToken: opts.PageToken,
			})
			if err != nil {
				fmt.Printf("ListResults: %v\n", err)
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
