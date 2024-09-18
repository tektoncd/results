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
	"strings"

	httpbody "google.golang.org/genproto/googleapis/api/httpbody"
	grpc "google.golang.org/grpc"

	"github.com/spf13/cobra"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/log"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/result"
	"github.com/tektoncd/results/pkg/cli/config"
	"github.com/tektoncd/results/pkg/cli/flags"
	"github.com/tektoncd/results/pkg/cli/format"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	pb3 "github.com/tektoncd/results/proto/v1alpha3/results_go_proto"
)

// GetLogCommand returns a cobra sub command that will fetch a log by name
func GetLogCommand(params *flags.Params) *cobra.Command {
	opts := &flags.GetOptions{}

	cmd := &cobra.Command{
		Use: "get [flags] <log-name>",

		Short: "Get Log by <log-name>",
		Long:  "Get Log by <log-name>. <log-name> is typically of format <namespace>/results/<parent-run-uuid>/logs/<child-run-uuid>",
		RunE: func(cmd *cobra.Command, args []string) error {
			var resp grpc.ServerStreamingClient[httpbody.HttpBody]
			var err error

			if config.GetConfig().UseV1Alpha2 {
				resp, err = params.LogsClient.GetLog(cmd.Context(), &pb.GetLogRequest{
					Name: args[0],
				})
			} else {
				var name, parent, res, rec string
				parent, res, rec, err = record.ParseName(args[0])
				if err != nil {
					if !strings.Contains(args[0], "logs") {
						return err
					}
					fmt.Printf("GetLog you can also pass in the format <namespace>/results/<parent-run-uuid>/records/<child-run-uuid>\n")
					name = args[0]
				} else {
					name = log.FormatName(result.FormatName(parent, res), rec)
				}
				resp, err = params.PluginLogsClient.GetLog(cmd.Context(), &pb3.GetLogRequest{
					Name: name,
				})
			}
			if err != nil {
				fmt.Printf("GetLog: %v\n", err)
				return err
			}
			data, err := resp.Recv()
			if err != nil {
				fmt.Printf("Get Log Client Resp: %v\n", err)
				return err
			}
			return format.PrintProto(os.Stdout, data, opts.Format)
		},
		Args: cobra.ExactArgs(1),
		Annotations: map[string]string{
			"commandType": "main",
		},
		Example: `  Lets assume, there is a PipelineRun in 'default' namespace (parent) with:
  PipelineRun UUID: 0dfc883d-722a-4489-9ab8-3cccc74ca4f6 (parent)
  TaskRun 1 UUID: db6a5d59-2170-3367-9eb5-83f3d264ec62 (child 1)
  TaskRun 2 UUID: 9514f318-9329-485b-871c-77a4a6904891 (child 2)

  - Get the log for TaskRun 1:
    tkn-results logs get default/results/0dfc883d-722a-4489-9ab8-3cccc74ca4f6/logs/db6a5d59-2170-3367-9eb5-83f3d264ec62
  
  - Get log for TaskRun 2:
    tkn-results logs get default/results/0dfc883d-722a-4489-9ab8-3cccc74ca4f6/logs/9514f318-9329-485b-871c-77a4a6904891
  
  - Get log for the PipelineRun:
    tkn-results logs get default/results/0dfc883d-722a-4489-9ab8-3cccc74ca4f6/logs/0dfc883d-722a-4489-9ab8-3cccc74ca4f6`,
	}

	flags.AddGetFlags(opts, cmd)

	return cmd
}
