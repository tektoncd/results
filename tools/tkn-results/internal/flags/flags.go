package flags

import (
	"github.com/spf13/cobra"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

type Params struct {
	ResultsClient pb.ResultsClient
	LogsClient    pb.LogsClient
}

// ListOptions is used on commands that list Results or Records
type ListOptions struct {
	Filter    string
	Limit     int32
	PageToken string
	Format    string
}

// AddListFlags is a helper function that adds common flags for commands that list things
func AddListFlags(options *ListOptions, cmd *cobra.Command) {
	cmd.Flags().StringVarP(&options.Filter, "filter", "f", "", "CEL Filter")
	cmd.Flags().Int32VarP(&options.Limit, "limit", "l", 0, "number of items to return. Response may be truncated due to server limits.")
	cmd.Flags().StringVarP(&options.PageToken, "page", "p", "", "pagination token to use for next page")
	cmd.Flags().StringVarP(&options.Format, "output", "o", "tab", "output format. Valid values: tab|textproto|json")
}

// GetOptions used on commands that list thing
type GetOptions struct {
	Format string
}

// AddGetFlags is a helper function that adds common flags for commands that list things
func AddGetFlags(options *GetOptions, cmd *cobra.Command) {
	cmd.Flags().StringVarP(&options.Format, "output", "o", "json", "output format. Valid values: textproto|json")
}
