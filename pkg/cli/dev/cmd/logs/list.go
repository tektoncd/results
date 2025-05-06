// Copyright 2023 The Tekton Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logs

import (
	"fmt"
	"os"

	"github.com/tektoncd/results/pkg/cli/dev/flags"
	"github.com/tektoncd/results/pkg/cli/dev/format"

	"github.com/spf13/cobra"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

// ListLogsCommand returns a cobra sub command that fetch a list of logs given the parent and result name
func ListLogsCommand(params *flags.Params) *cobra.Command {
	opts := &flags.ListOptions{}

	cmd := &cobra.Command{
		Use:   "list [flags] <result-name>",
		Short: "[To be deprecated] List Logs for a given Result",
		Long:  "List Logs for a given Result. <result-name> is typically of format <namespace>/results/<parent-run-uuid>. '-' may be used in place of <parent-run-uuid> to query all Logs for a given parent.",
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := params.LogsClient.ListLogs(cmd.Context(), &pb.ListRecordsRequest{
				Parent:    args[0],
				Filter:    opts.Filter,
				PageSize:  opts.Limit,
				PageToken: opts.PageToken,
			})
			if err != nil {
				fmt.Printf("List Logs: %v\n", err)
				return err
			}
			return format.PrintProto(os.Stdout, resp, opts.Format)
		},
		Args: cobra.ExactArgs(1),
		Annotations: map[string]string{
			"commandType": "main",
		},
		Example: `  - List all Logs for PipelineRun with UUID 0dfc883d-722a-4489-9ab8-3cccc74ca4f6 in 'default' namespace:
    tkn-results logs list default/results/0dfc883d-722a-4489-9ab8-3cccc74ca4f6
  - List all logs for all Runs in 'default' namespace:
    tkn-results logs list default/results/-
  - List only TaskRuns logs in 'default' namespace:
    tkn-results logs list default/results/- --filter="data.spec.resource.kind=TaskRun"`,
	}

	flags.AddListFlags(opts, cmd)

	return cmd
}
