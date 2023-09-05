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

package records

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tektoncd/results/pkg/cli/flags"
	"github.com/tektoncd/results/pkg/cli/format"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

func GetRecordCommand(params *flags.Params) *cobra.Command {
	opts := &flags.GetOptions{}

	cmd := &cobra.Command{
		Use: `get [flags] <record_path>

  <record name>: Fully qualified name of the record. This is typically "<namespace>/results/<result name>/records/<record uid>".`,
		Short: "Get Record",
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := params.ResultsClient.GetRecord(cmd.Context(), &pb.GetRecordRequest{
				Name: args[0],
			})
			if err != nil {
				fmt.Printf("GetRecord: %v\n", err)
				return err
			}
			return format.PrintProto(os.Stdout, resp, opts.Format)
		},
		Args: cobra.ExactArgs(1),
		Annotations: map[string]string{
			"commandType": "main",
		},
	}

	flags.AddGetFlags(opts, cmd)

	return cmd
}
