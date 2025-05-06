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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"text/template"

	"github.com/tektoncd/results/pkg/cli/dev/flags"
	"github.com/tektoncd/results/pkg/cli/dev/format"

	"github.com/jonboulle/clockwork"
	"github.com/spf13/cobra"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

const tmpl = `Name:              {{.Record.Name}}
UID:               {{.Record.Uid}}
Status:
Created:	{{ formatAge .Record.CreateTime .Time }}	Duration:	{{formatDuration .Record.CreateTime .Record.UpdateTime}}
{{- if .Record.Data }}
Type:         {{.Record.Data.Type}}
Data:
{{formatJSON .Record.Data.Value}}
{{ end -}}
`

// GetRecordCommand returns a cobra sub command that will fetch a record by name
func GetRecordCommand(params *flags.Params) *cobra.Command {
	opts := &flags.GetOptions{}

	cmd := &cobra.Command{
		Use:   "get [flags] <record-name>",
		Short: "[To be deprecated] Get Record by <record-name>",
		Long:  "Get Record by <record-name>. <record-name> is typically of format <namespace>/results/<parent-run-uuid>/records/<child-run-uuid>",
		RunE: func(cmd *cobra.Command, args []string) error {
			record, err := params.ResultsClient.GetRecord(cmd.Context(), &pb.GetRecordRequest{
				Name: args[0],
			})
			if err != nil {
				fmt.Printf("GetRecord: %v\n", err)
				return err
			}
			return formatRecord(cmd.OutOrStdout(), record, params.Clock)
		},
		Args: cobra.ExactArgs(1),
		Annotations: map[string]string{
			"commandType": "main",
		},
		Example: `  Lets assume, there is a PipelineRun in 'default' namespace (parent) with:
  PipelineRun UUID: 0dfc883d-722a-4489-9ab8-3cccc74ca4f6 (parent)
  TaskRun 1 UUID: db6a5d59-2170-3367-9eb5-83f3d264ec62 (child 1)
  TaskRun 2 UUID: 9514f318-9329-485b-871c-77a4a6904891 (child 2)

  - Get the record for TaskRun 1:
    tkn-results records get default/results/0dfc883d-722a-4489-9ab8-3cccc74ca4f6/records/db6a5d59-2170-3367-9eb5-83f3d264ec62

  - Get the record for TaskRun 2:
    tkn-results records get default/results/0dfc883d-722a-4489-9ab8-3cccc74ca4f6/records/9514f318-9329-485b-871c-77a4a6904891

  - Get the record for PipelineRun:
    tkn-results records get default/results/0dfc883d-722a-4489-9ab8-3cccc74ca4f6/records/0dfc883d-722a-4489-9ab8-3cccc74ca4f6`,
	}

	flags.AddGetFlags(opts, cmd)

	return cmd
}

func formatRecord(out io.Writer, record *pb.Record, c clockwork.Clock) error {
	data := struct {
		Record *pb.Record
		Time   clockwork.Clock
	}{
		Record: record,
		Time:   c,
	}

	funcMap := template.FuncMap{
		"formatAge":      format.Age,
		"formatDuration": format.Duration,
		"formatJSON": func(data []byte) string {
			if len(data) == 0 {
				return "No data"
			}
			var prettyJSON bytes.Buffer
			if err := json.Indent(&prettyJSON, data, "       ", "  "); err != nil {
				return fmt.Sprintf("Error formatting JSON: %v", err)
			}
			return prettyJSON.String()
		},
	}
	w := tabwriter.NewWriter(out, 0, 5, 3, ' ', tabwriter.TabIndent)
	t := template.Must(template.New("record").Funcs(funcMap).Parse(tmpl))
	return t.Execute(w, data)
}
