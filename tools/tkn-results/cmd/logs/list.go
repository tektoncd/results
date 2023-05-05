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

	"github.com/spf13/cobra"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"github.com/tektoncd/results/tools/tkn-results/internal/flags"
	"github.com/tektoncd/results/tools/tkn-results/internal/format"
)

func ListCommand(params *flags.Params) *cobra.Command {
	opts := &flags.ListOptions{}

	cmd := &cobra.Command{
		Use: `list [flags] <result parent>

  <result parent>: Result parent name to query. This is typically "<namespace>/results/<result name>", but may vary depending on the API Server. "-" may be used as <result name> to query all Logs for a given parent.`,
		Short: "List Logs",
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
	}

	flags.AddListFlags(opts, cmd)

	return cmd
}
