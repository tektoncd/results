package pipelinerun

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"text/template"

	"github.com/jonboulle/clockwork"
	"github.com/spf13/cobra"
	"github.com/tektoncd/cli/pkg/formatted"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	pipelinev1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/results/pkg/cli/flags"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

const listTemplate = `{{- $size := len .PipelineRuns -}}{{- if eq $size 0 -}}
No PipelineRuns found
{{ else -}}
NAMESPACE	UID	STARTED	DURATION	STATUS
{{- range $_, $pr := .PipelineRuns }}
{{ $pr.ObjectMeta.Namespace }}	{{ $pr.ObjectMeta.Name }}	{{ formatAge $pr.Status.StartTime $.Time }}	{{ formatDuration $pr.Status.StartTime $pr.Status.CompletionTime }}	{{ formatCondition $pr.Status.Conditions }}
{{- end -}}
{{- end -}}`

type listOptions struct {
	Namespace string
	Limit     int32
}

// listCommand initializes a cobra command to list PipelineRuns
func listCommand(params *flags.Params) *cobra.Command {
	opts := &listOptions{Limit: 0, Namespace: "default"}

	eg := `List all PipelineRuns in a namespace 'foo':
    tkn-results pipelinerun list -n foo

List all PipelineRuns in 'default' namespace:
    tkn-results pipelinerun list -n default
`
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List PipelineRuns in a namespace",
		Annotations: map[string]string{
			"commandType": "main",
		},
		Example: eg,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if opts.Limit < 0 {
				return fmt.Errorf("limit was %d, but must be greater than 0", opts.Limit)
			}

			resp, err := params.ResultsClient.ListRecords(cmd.Context(), &pb.ListRecordsRequest{
				Parent:   fmt.Sprintf("%s/results/-", opts.Namespace),
				PageSize: opts.Limit,
				Filter:   `data_type==PIPELINE_RUN`,
			})
			if err != nil {
				return fmt.Errorf("failed to list PipelineRuns from namespace %s: %v", opts.Namespace, err)
			}
			return printFormatted(cmd.OutOrStdout(), resp.Records, params.Clock)
		},
	}
	cmd.Flags().StringVarP(&opts.Namespace, "namespace", "n", "default", "Namespace to list PipelineRuns in")
	cmd.Flags().Int32VarP(&opts.Limit, "limit", "l", 0, "Limit the number of PipelineRuns to return")
	return cmd
}

func pipelineRunFromRecord(record *pb.Record) (*pipelinev1.PipelineRun, error) {
	if record.Data == nil {
		return nil, fmt.Errorf("record data is nil")
	}
	pr := &pipelinev1.PipelineRun{}
	switch record.Data.GetType() {
	case "tekton.dev/v1beta1.PipelineRun":
		//nolint:staticcheck
		prV1beta1 := &pipelinev1beta1.PipelineRun{}
		if err := json.Unmarshal(record.Data.Value, prV1beta1); err != nil {
			return nil, fmt.Errorf("failed to unmarshal PipelineRun data: %v", err)
		}
		if err := pr.ConvertFrom(context.TODO(), prV1beta1); err != nil {
			return nil, fmt.Errorf("failed to convert v1beta1 PipelineRun to v1: %v", err)
		}
	case "tekton.dev/v1.PipelineRun":
		if err := json.Unmarshal(record.Data.Value, pr); err != nil {
			return nil, fmt.Errorf("failed to unmarshal PipelineRun data: %v", err)
		}
	default:
		return nil, fmt.Errorf("unsupported PipelineRun type: %s", record.Data.GetType())
	}
	return pr, nil
}

func printFormatted(out io.Writer, records []*pb.Record, c clockwork.Clock) error {
	var data = struct {
		PipelineRuns []*pipelinev1.PipelineRun
		Time         clockwork.Clock
	}{
		PipelineRuns: []*pipelinev1.PipelineRun{},
		Time:         c,
	}

	for _, record := range records {
		if pr, err := pipelineRunFromRecord(record); err == nil {
			data.PipelineRuns = append(data.PipelineRuns, pr)
		}
	}

	funcMap := template.FuncMap{
		"formatAge":       formatted.Age,
		"formatDuration":  formatted.Duration,
		"formatCondition": formatted.Condition,
	}

	w := tabwriter.NewWriter(out, 0, 5, 3, ' ', tabwriter.TabIndent)
	t := template.Must(template.New("List TaskRuns").Funcs(funcMap).Parse(listTemplate))

	err := t.Execute(w, data)
	if err != nil {
		return err
	}
	return w.Flush()
}
