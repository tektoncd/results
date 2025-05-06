package result

import (
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

const resultDescTmpl = `Name:	{{.Result.Name}}
UID:	{{.Result.Uid}}
{{- if .Result.Annotations}}
Annotations:
{{- range $key, $value := .Result.Annotations}}
	{{$key}}={{$value}}
{{- end}}
{{- end}}
Status:
	Created:	{{formatAge .Result.CreateTime .Time}}	DURATION: {{formatDuration .Result.CreateTime .Result.UpdateTime}}
{{- if .Result.Summary}}
Summary:
	Type:	{{.Result.Summary.Type}}
	Status:
	STARTED	DURATION	STATUS
	{{formatAge .Result.Summary.StartTime .Time}}	{{formatDuration .Result.Summary.StartTime .Result.Summary.EndTime}}	{{.Result.Summary.Status}}
	{{- if .Result.Summary.Annotations}}
	Annotations:
		{{- range $key, $value := .Result.Summary.Annotations}}
			{{$key}}={{$value}}
		{{- end}}
	{{- end}}
{{- end}}
`

type describeOptions struct {
	Parent string
	UID    string
}

func describeCommand(params *flags.Params) *cobra.Command {
	opts := &describeOptions{}
	eg := `Query by name:
tkn-results result describe default/results/e6b4b2e3-d876-4bbe-a927-95c691b6fdc7

Query by parent and uid:
tkn-results result desc --parent default --uid 949eebd9-1cf7-478f-a547-9ee313035f10
`
	cmd := &cobra.Command{
		Use:     "describe [-p parent -u uid] [name]",
		Aliases: []string{"desc"},
		Short:   "[To be deprecated] Describes a Result",
		Annotations: map[string]string{
			"commandType": "main",
		},
		Example: eg,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				name := args[0]
				result, err := params.ResultsClient.GetResult(cmd.Context(), &pb.GetResultRequest{
					Name: name,
				})
				if err != nil {
					return fmt.Errorf("failed to get result of name %s: %v", name, err)
				}
				return printResultDescription(cmd.OutOrStdout(), result, params.Clock)
			}

			if opts.Parent != "" && opts.UID != "" {
				resp, err := params.ResultsClient.ListResults(cmd.Context(), &pb.ListResultsRequest{
					Parent: opts.Parent,
					Filter: fmt.Sprintf(`uid=="%s"`, opts.UID),
				})
				if err != nil {
					return fmt.Errorf("failed to get result of parent %s and uid %s: %v", opts.Parent, opts.UID, err)
				}
				if len(resp.Results) == 0 {
					return fmt.Errorf("no result found with parent %s and uid %s", opts.Parent, opts.UID)
				}
				return printResultDescription(cmd.OutOrStdout(), resp.Results[0], params.Clock)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&opts.Parent, "parent", "p", "", "[To be deprecated] parent of the result")
	cmd.Flags().StringVarP(&opts.UID, "uid", "u", "", "[To be deprecated] uid of the result")
	return cmd
}

func printResultDescription(out io.Writer, result *pb.Result, c clockwork.Clock) error {
	data := struct {
		Result *pb.Result
		Time   clockwork.Clock
	}{
		Result: result,
		Time:   c,
	}
	funcMap := template.FuncMap{
		"formatAge":      format.Age,
		"formatDuration": format.Duration,
		"formatStatus":   format.Status,
	}
	w := tabwriter.NewWriter(out, 0, 5, 3, ' ', tabwriter.TabIndent)
	t := template.Must(template.New("Describe A Result").Funcs(funcMap).Parse(resultDescTmpl))
	err := t.Execute(w, data)
	if err != nil {
		return err
	}
	return w.Flush()
}
