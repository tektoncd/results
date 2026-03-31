// Package pipelinerun provides CLI commands for PipelineRun resources.
package pipelinerun

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"text/template"

	"github.com/tektoncd/results/pkg/cli/options"

	"github.com/tektoncd/results/pkg/cli/client/records"

	"github.com/jonboulle/clockwork"
	"github.com/spf13/cobra"
	"github.com/tektoncd/cli/pkg/cli"
	"github.com/tektoncd/cli/pkg/formatted"
	v1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tektoncd/results/pkg/cli/common"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"

	"k8s.io/cli-runtime/pkg/printers"
)

const describeTemplate = `{{decorate "bold" "Name"}}: {{ .PipelineRun.Name }}
{{decorate "bold" "Namespace"}}: {{ .PipelineRun.Namespace }}
{{- if .PipelineRun.Spec.PipelineRef }}
{{- if ne .PipelineRun.Spec.PipelineRef.Name "" }}
{{decorate "bold" "Pipeline Ref"}}: {{ .PipelineRun.Spec.PipelineRef.Name }}
{{- end }}
{{- end }}
{{- if .PipelineRun.Spec.TaskRunTemplate.ServiceAccountName }}
{{decorate "bold" "Service Account"}}: {{ .PipelineRun.Spec.TaskRunTemplate.ServiceAccountName }}
{{- end }}
{{- $l := len .PipelineRun.Labels }}{{ if eq $l 0 }}
{{- else }}
{{decorate "bold" "Labels"}}:
{{- range $k, $v := .PipelineRun.Labels }}
 {{ $k }}={{ $v }}
{{- end }}
{{- end }}
{{- if .PipelineRun.Annotations }}
{{decorate "bold" "Annotations"}}:
{{- range $k, $v := .PipelineRun.Annotations }}
 {{ $k }}={{ $v }}
{{- end }}
{{- end }}

📌 {{decorate "underline bold" "Status"}}
STARTED          DURATION         STATUS
{{ formatAge .PipelineRun.Status.StartTime .Time | printf "%-16s" }} {{ formatDuration .PipelineRun.Status.StartTime .PipelineRun.Status.CompletionTime | printf "%-16s" }} {{ formatCondition .PipelineRun.Status.Conditions }}

⏱ {{decorate "underline bold" "Timeouts"}}
{{- if .PipelineRun.Spec.Timeouts }}
{{- if .PipelineRun.Spec.Timeouts.Pipeline }}
Pipeline:   {{ .PipelineRun.Spec.Timeouts.Pipeline.Duration.String }}
{{- end }}
{{- if .PipelineRun.Spec.Timeouts.Tasks }}
Tasks:      {{ .PipelineRun.Spec.Timeouts.Tasks.Duration.String }}
{{- end }}
{{- if .PipelineRun.Spec.Timeouts.Finally }}
Finally:    {{ .PipelineRun.Spec.Timeouts.Finally.Duration.String }}
{{- end }}
{{- end }}

⚓ {{decorate "underline bold" "Params"}}
{{- if ne (len .PipelineRun.Spec.Params) 0 }}
  NAME                          VALUE
{{- range $i, $p := .PipelineRun.Spec.Params }}
  • {{ $p.Name | printf "% -28s" }}{{ if eq $p.Value.Type "string" }}{{ $p.Value.StringVal }}{{ else if eq $p.Value.Type "array" }}{{ $p.Value.ArrayVal }}{{ else }}{{ $p.Value.ObjectVal }}{{ end }}
{{- end }}
{{- end }}

{{- if ne (len .PipelineRun.Status.Results) 0 }}
📝 {{decorate "underline bold" "Results"}}
  NAME                          VALUE
{{- range $result := .PipelineRun.Status.Results }}
  • {{ $result.Name | printf "% -28s" }}{{ if eq $result.Value.Type "string" }}{{ $result.Value.StringVal }}{{ else if eq $result.Value.Type "array" }}{{ $result.Value.ArrayVal }}{{ else }}{{ $result.Value.ObjectVal }}{{ end }}
{{- end }}
{{- end }}

{{- if ne (len .PipelineRun.Spec.Workspaces) 0 }}

🗂  {{decorate "underline bold" "Workspaces"}}
  NAME                SUB PATH            WORKSPACE BINDING
{{- range $workspace := .PipelineRun.Spec.Workspaces }}
  • {{ $workspace.Name | printf "% -19s" }}{{ if not $workspace.SubPath }}{{ "---" | printf "% -19s" }}{{ else }}{{ $workspace.SubPath | printf "% -19s" }}{{ end }}{{ formatWorkspace $workspace }}
{{- end }}
{{- end }}

{{- if ne (len .PipelineRun.Status.ChildReferences) 0 }}

📦 {{decorate "underline bold" "Taskruns"}}
  NAME                                                                         TASK NAME
{{- range $taskrun := .PipelineRun.Status.ChildReferences }}
  • {{ $taskrun.Name | printf "% -75s" }}{{ $taskrun.PipelineTaskName }}
{{- end }}
{{- end }}

{{- if ne (len .PipelineRun.Status.SkippedTasks) 0 }}
⏭️  {{decorate "underline bold" "Skipped Tasks"}}
NAME
{{- range $skippedTask := .PipelineRun.Status.SkippedTasks }}
• {{ $skippedTask.Name }}
{{- end }}
{{- end }}
`

// describeCommand initializes a cobra command to describe a PipelineRun
func describeCommand(p common.Params) *cobra.Command {
	opts := &options.DescribeOptions{
		ResourceType: common.ResourceTypePipelineRun,
	}

	var outputFormat string

	eg := `Describe a PipelineRun in namespace 'foo':
    tkn-results pipelinerun describe my-pipelinerun -n foo

Describe a PipelineRun in the current namespace:
    tkn-results pipelinerun describe my-pipelinerun

Describe a PipelineRun as yaml:
    tkn-results pipelinerun describe my-pipelinerun -o yaml

Describe a PipelineRun as json:
    tkn-results pipelinerun describe my-pipelinerun -o json
`
	cmd := &cobra.Command{
		Use:     "describe [pipelinerun-name]",
		Aliases: []string{"desc"},
		Short:   "Describe a PipelineRun",
		Long: `Describe a PipelineRun by name or UID. If --uid is provided, then PipelineRun name is optional.

If multiple PipelineRuns match the given name, the most recent one is returned.
Use --uid to target a specific PipelineRun when needed.`,
		Annotations: map[string]string{
			"commandType": "main",
		},
		Example: eg,
		Args: func(_ *cobra.Command, args []string) error {
			// If UID is provided, no arguments are required
			if opts.UID != "" {
				return nil
			}
			// Otherwise, require exactly one argument
			if len(args) != 1 {
				return fmt.Errorf("requires exactly one argument when --uid is not provided")
			}
			return nil
		},
		PreRunE: func(_ *cobra.Command, args []string) error {
			// Initialize the client using the shared prerun function
			opts.Client = p.RESTClient()
			if len(args) > 0 {
				opts.ResourceName = args[0]
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			recordClient := records.NewClient(opts.Client)

			var record *pb.Record

			if opts.UID != "" {
				// Direct primary key lookup by UID
				r, err := recordClient.GetRecord(ctx, p.Namespace(), opts.UID)
				if err == nil {
					record = r
				} else {
					// Fallback: filter by record name column (text, indexed)
					// instead of data.metadata.uid (JSONB, unindexed).
					filter := fmt.Sprintf(`name=="%s"`, opts.UID)
					parent := fmt.Sprintf("%s/results/-", p.Namespace())
					resp, err := recordClient.ListRecords(ctx, &pb.ListRecordsRequest{
						Parent:   parent,
						Filter:   filter,
						OrderBy:  "create_time desc",
						PageSize: 5,
					}, "")
					if err != nil {
						return fmt.Errorf("failed to find PipelineRun: %v", err)
					}
					if len(resp.Records) == 0 {
						if opts.ResourceName != "" {
							return fmt.Errorf("no PipelineRun found with name %s and UID %s", opts.ResourceName, opts.UID)
						}
						return fmt.Errorf("no PipelineRun found with UID %s", opts.UID)
					}
					record = resp.Records[0]
				}
			} else {
				filter := common.BuildFilterString(opts)
				parent := fmt.Sprintf("%s/results/-", p.Namespace())
				resp, err := recordClient.ListRecords(ctx, &pb.ListRecordsRequest{
					Parent:   parent,
					Filter:   filter,
					OrderBy:  "create_time desc",
					PageSize: 5,
				}, "")
				if err != nil {
					return fmt.Errorf("failed to find PipelineRun: %v", err)
				}
				if len(resp.Records) == 0 {
					return fmt.Errorf("no PipelineRun found with name %s", opts.ResourceName)
				}
				record = resp.Records[0]
			}

			var pr v1.PipelineRun
			if err := json.Unmarshal(record.Data.Value, &pr); err != nil {
				return fmt.Errorf("failed to unmarshal PipelineRun data: %v", err)
			}

			// Output as json or yaml if requested
			if outputFormat == "json" {
				printer := &printers.JSONPrinter{}
				return printer.PrintObj(&pr, cmd.OutOrStdout())
			}
			if outputFormat == "yaml" {
				printer := &printers.YAMLPrinter{}
				return printer.PrintObj(&pr, cmd.OutOrStdout())
			}

			// Print formatted output
			stream := &cli.Stream{
				Out: cmd.OutOrStdout(),
				Err: cmd.OutOrStderr(),
			}

			var data = struct {
				PipelineRun *v1.PipelineRun
				Time        clockwork.Clock
			}{
				PipelineRun: &pr,
				Time:        clockwork.NewRealClock(),
			}

			funcMap := template.FuncMap{
				"formatAge":       common.FormatAge,
				"formatDuration":  formatted.Duration,
				"formatCondition": formatted.Condition,
				"formatWorkspace": formatted.Workspace,
				"decorate":        formatted.DecorateAttr,
			}

			w := tabwriter.NewWriter(stream.Out, 0, 5, 3, ' ', tabwriter.TabIndent)
			t := template.Must(template.New("Describe PipelineRun").Funcs(funcMap).Parse(describeTemplate))

			if err := t.Execute(w, data); err != nil {
				return err
			}

			return w.Flush()
		},
	}

	cmd.Flags().StringVar(&opts.UID, "uid", "", "UID of the PipelineRun to describe")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "", "Output format. One of: json|yaml (Default format is used if not specified)")

	return cmd
}
