package taskrun

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	"text/template"

	"github.com/jonboulle/clockwork"
	"github.com/spf13/cobra"
	"github.com/tektoncd/cli/pkg/cli"
	"github.com/tektoncd/cli/pkg/formatted"
	v1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tektoncd/results/pkg/cli/client/records"
	"github.com/tektoncd/results/pkg/cli/common"
	"github.com/tektoncd/results/pkg/cli/common/prerun"
	"github.com/tektoncd/results/pkg/cli/options"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"

	"k8s.io/cli-runtime/pkg/printers"
)

const describeTemplate = `{{decorate "bold" "Name"}}: {{ .TaskRun.Name }}
{{decorate "bold" "Namespace"}}: {{ .TaskRun.Namespace }}
{{- if .TaskRun.Spec.ServiceAccountName }}
{{decorate "bold" "Service Account"}}: {{ .TaskRun.Spec.ServiceAccountName }}
{{- end }}
{{- $l := len .TaskRun.Labels }}{{ if eq $l 0 }}
{{- else }}
{{decorate "bold" "Labels"}}:
{{- range $k, $v := .TaskRun.Labels }}
 {{ $k }}={{ $v }}
{{- end }}
{{- end }}
{{- if .TaskRun.Annotations }}
{{decorate "bold" "Annotations"}}:
{{- range $k, $v := .TaskRun.Annotations }}
 {{ $k }}={{ $v }}
{{- end }}
{{- end }}

📌 {{decorate "underline bold" "Status"}}
STARTED          DURATION         STATUS
{{ formatAge .TaskRun.Status.StartTime .Time | printf "% -16s" }} {{ formatDuration .TaskRun.Status.StartTime .TaskRun.Status.CompletionTime | printf "% -16s" }} {{ formatCondition .TaskRun.Status.Conditions }}

⚓ {{decorate "underline bold" "Params"}}
{{- if ne (len .TaskRun.Spec.Params) 0 }}
  NAME                          VALUE
{{- range $i, $p := .TaskRun.Spec.Params }}
  • {{ $p.Name | printf "% -28s" }}{{ if eq $p.Value.Type "string" }}{{ $p.Value.StringVal }}{{ else if eq $p.Value.Type "array" }}{{ $p.Value.ArrayVal }}{{ else }}{{ $p.Value.ObjectVal }}{{ end }}
{{- end }}
{{- end }}

{{- if ne (len .TaskRun.Status.Results) 0 }}

📝 {{decorate "underline bold" "Results"}}
  NAME                          VALUE
{{- range $result := .TaskRun.Status.Results }}
  • {{ $result.Name | printf "% -28s" }}{{ if eq $result.Value.Type "string" }}{{ $result.Value.StringVal }}{{ else if eq $result.Value.Type "array" }}{{ $result.Value.ArrayVal }}{{ else }}{{ $result.Value.ObjectVal }}{{ end }}
{{- end }}
{{- end }}

{{- if ne (len .TaskRun.Spec.Workspaces) 0 }}

🗂  {{decorate "underline bold" "Workspaces"}}
  NAME                SUB PATH            WORKSPACE BINDING
{{- range $workspace := .TaskRun.Spec.Workspaces }}
  • {{ $workspace.Name | printf "% -19s" }}{{ if not $workspace.SubPath }}{{ "---" | printf "% -19s" }}{{ else }}{{ $workspace.SubPath | printf "% -19s" }}{{ end }}{{ formatWorkspace $workspace }}
{{- end }}
{{- end }}
`

func describeCommand(p common.Params) *cobra.Command {
	opts := &options.DescribeOptions{
		ResourceType: common.ResourceTypeTaskRun,
	}

	var outputFormat string

	eg := `Describe a TaskRun in namespace 'foo':
    tkn-results taskrun describe my-taskrun -n foo

Describe a TaskRun in the current namespace
    tkn-results taskrun describe my-taskrun

Describe a TaskRun as yaml
    tkn-results taskrun describe my-taskrun -o yaml

Describe a TaskRun as json
    tkn-results taskrun describe my-taskrun -o json
`
	cmd := &cobra.Command{
		Use:     "describe [taskrun-name]",
		Aliases: []string{"desc"},
		Short:   "Describe a TaskRun",
		Long:    "Describe a TaskRun by name or UID. If --uid is provided, then TaskRun name is optional.",
		Annotations: map[string]string{
			"commandType": "main",
		},
		Example: eg,
		Args: func(_ *cobra.Command, args []string) error {
			if opts.UID != "" {
				return nil
			}
			if len(args) != 1 {
				return fmt.Errorf("requires exactly one argument when --uid is not provided")
			}
			return nil
		},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			var err error
			opts.Client, err = prerun.InitClient(p, cmd)
			if err != nil {
				return err
			}
			if len(args) > 0 {
				opts.ResourceName = args[0]
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			filter := common.BuildFilterString(opts)
			parent := fmt.Sprintf("%s/results/-", p.Namespace())

			recordClient := records.NewClient(opts.Client)
			resp, err := recordClient.ListRecords(ctx, &pb.ListRecordsRequest{
				Parent:   parent,
				Filter:   filter,
				PageSize: 10,
			}, "")

			if err != nil {
				return fmt.Errorf("failed to find TaskRun: %v", err)
			}
			if len(resp.Records) == 0 {
				if opts.UID != "" && opts.ResourceName != "" {
					return fmt.Errorf("no TaskRun found with name %s and UID %s", opts.ResourceName, opts.UID)
				} else if opts.UID != "" {
					return fmt.Errorf("no TaskRun found with UID %s", opts.UID)
				}
				return fmt.Errorf("no TaskRun found with name %s", opts.ResourceName)
			}
			if len(resp.Records) > 1 {
				var uids []string
				for _, record := range resp.Records {
					uids = append(uids, record.Uid)
				}
				return fmt.Errorf("multiple TaskRuns found. Use a more specific name or UID. Available UIDs are: %s",
					strings.Join(uids, ", "))
			}

			var tr v1.TaskRun
			if err := json.Unmarshal(resp.Records[0].Data.Value, &tr); err != nil {
				return fmt.Errorf("failed to unmarshal TaskRun data: %v", err)
			}

			if outputFormat == "json" {
				printer := &printers.JSONPrinter{}
				return printer.PrintObj(&tr, cmd.OutOrStdout())
			}
			if outputFormat == "yaml" {
				printer := &printers.YAMLPrinter{}
				return printer.PrintObj(&tr, cmd.OutOrStdout())
			}

			stream := &cli.Stream{
				Out: cmd.OutOrStdout(),
				Err: cmd.OutOrStderr(),
			}

			var data = struct {
				TaskRun *v1.TaskRun
				Time    clockwork.Clock
			}{
				TaskRun: &tr,
				Time:    clockwork.NewRealClock(),
			}

			funcMap := template.FuncMap{
				"formatAge":       common.FormatAge,
				"formatDuration":  formatted.Duration,
				"formatCondition": formatted.Condition,
				"formatWorkspace": formatted.Workspace,
				"decorate":        formatted.DecorateAttr,
			}

			w := tabwriter.NewWriter(stream.Out, 0, 5, 3, ' ', tabwriter.TabIndent)
			t := template.Must(template.New("Describe TaskRun").Funcs(funcMap).Parse(describeTemplate))

			if err := t.Execute(w, data); err != nil {
				return err
			}

			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&opts.UID, "uid", "", "UID of the TaskRun to describe")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "", "Output format. One of: json|yaml (Default format is used if not specified)")

	return cmd
}
