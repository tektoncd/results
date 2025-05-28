package taskrun

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"text/template"

	"github.com/tektoncd/results/pkg/cli/options"

	"github.com/jonboulle/clockwork"
	"github.com/spf13/cobra"
	"github.com/tektoncd/cli/pkg/cli"
	"github.com/tektoncd/cli/pkg/formatted"
	v1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tektoncd/results/pkg/cli/client/records"
	"github.com/tektoncd/results/pkg/cli/common"
	"github.com/tektoncd/results/pkg/cli/common/prerun"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

const trListTemplate = `{{- $trl := len .TaskRuns.Items -}}{{- if eq $trl 0 -}}
No TaskRuns found
{{ else -}}
{{- if not $.NoHeaders -}}
{{- if $.AllNamespaces -}}
NAMESPACE	NAME	UID	STARTED	DURATION	STATUS
{{ else -}}
NAME	UID	STARTED	DURATION	STATUS
{{ end -}}
{{- end -}}
{{- range $_, $tr := .TaskRuns.Items }}{{- if $tr }}{{- if $.AllNamespaces -}}
{{ $tr.Namespace }}	{{ $tr.Name }}	{{ $tr.UID }}	{{ formatAge $tr.Status.StartTime $.Time }}	{{ formatDuration $tr.Status.StartTime $tr.Status.CompletionTime }}	{{ formatCondition $tr.Status.Conditions }}
{{ else -}}
{{ $tr.Name }}	{{ $tr.UID }}	{{ formatAge $tr.Status.StartTime $.Time }}	{{ formatDuration $tr.Status.StartTime $tr.Status.CompletionTime }}	{{ formatCondition $tr.Status.Conditions }}
{{ end -}}{{- end -}}{{- end -}}
{{- end -}}`

// listCommand initializes a cobra command to list TaskRuns
func listCommand(p common.Params) *cobra.Command {
	opts := &options.ListOptions{
		Limit:         50, // Default to 50
		AllNamespaces: false,
		SinglePage:    true, // Default to true
		ResourceType:  common.ResourceTypeTaskRun,
	}

	eg := `List all TaskRuns in a namespace 'foo':
    tkn-results taskrun list -n foo

List TaskRuns with a specific label:
    tkn-results taskrun list -L app=myapp

List TaskRuns with multiple labels:
    tkn-results taskrun list --label app=myapp,env=prod

List TaskRuns from all namespaces:
    tkn-results taskrun list -A

List TaskRuns with limit of 20 per page:
    tkn-results taskrun list --limit=20

List TaskRuns for a specific task:
    tkn-results taskrun list foo -n namespace

List TaskRuns with partial task name match:
    tkn-results taskrun list build -n namespace

List TaskRuns for a specific PipelineRun:
    tkn-results taskrun list --pipelinerun my-pipeline-run -n namespace
`
	cmd := &cobra.Command{
		Use:     "list [task-name]",
		Aliases: []string{"ls"},
		Short:   "List TaskRuns in a namespace",
		Annotations: map[string]string{
			"commandType": "main",
		},
		Example: eg,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			allNs, _ := cmd.Flags().GetBool("all-namespaces")
			nsSet := cmd.Flags().Changed("namespace")
			if allNs && nsSet {
				return errors.New("cannot use --all-namespaces/-A and --namespace/-n together")
			}
			// Initialize the client using the shared prerun function
			var err error
			opts.Client, err = prerun.InitClient(p, cmd)
			if err != nil {
				return err
			}

			if opts.Limit < 5 || opts.Limit > 1000 {
				return errors.New("limit should be between 5 and 1000")
			}
			// Validate label format if provided
			if opts.Label != "" {
				return common.ValidateLabels(opts.Label)
			}
			if len(args) > 0 {
				opts.ResourceName = args[0]
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return listTaskRuns(cmd.Context(), p, opts)
		},
	}

	cmd.Flags().Int32VarP(&opts.Limit, "limit", "", 50, "Maximum number of TaskRuns to return (must be between 5 and 1000, default is 50)")
	cmd.Flags().BoolVarP(&opts.AllNamespaces, "all-namespaces", "A", false, "List TaskRuns from all namespaces")
	cmd.Flags().StringVarP(&opts.Label, "label", "L", "", "Filter by label (format: key=value[,key=value...])")
	cmd.Flags().StringVarP(&opts.PipelineRun, "pipelinerun", "", "", "Filter TaskRuns by PipelineRun name. Note that multiple PipelineRuns can have the same name, so this will return TaskRuns from all PipelineRuns with the matching name.")
	cmd.Flags().BoolVar(&opts.SinglePage, "single-page", true, "Return only a single page of results")

	return cmd
}

func listTaskRuns(ctx context.Context, p common.Params, opts *options.ListOptions) error {
	// Build filter string
	filter := common.BuildFilterString(opts)

	// Handle all namespaces
	parent := fmt.Sprintf("%s/results/-", p.Namespace())
	if opts.AllNamespaces {
		parent = common.AllNamespacesResultsParent
	}

	// Create initial request
	req := &pb.ListRecordsRequest{
		Parent:   parent,
		Filter:   filter,
		OrderBy:  "create_time desc",
		PageSize: opts.Limit,
	}

	// Create record client
	recordClient := records.NewClient(opts.Client)

	// Interactive pagination
	reader := bufio.NewReader(os.Stdin)
	for {
		resp, err := recordClient.ListRecords(ctx, req, common.ListFields)
		if err != nil {
			return err
		}

		stream := &cli.Stream{
			Out: os.Stdout,
			Err: os.Stderr,
		}

		// Parse records to TaskRuns before printing
		trs, err := parseRecordsToTr(resp.Records)
		if err != nil {
			return err
		}
		if err := printFormattedTr(stream, trs, clockwork.NewRealClock(), opts.AllNamespaces, false); err != nil {
			return err
		}

		// If single page is requested or if there's no next page token,
		// we're done break after first iteration
		if opts.SinglePage || resp.NextPageToken == "" {
			break
		}

		// Prompt for next page
		if _, err := fmt.Fprintf(stream.Out, "\nPress 'n' for next page, 'q' to quit: "); err != nil {
			return err
		}
		input, err := reader.ReadString('\n')
		if err != nil {
			return err
		}

		// Trim whitespace and convert to lowercase
		input = strings.TrimSpace(strings.ToLower(input))

		// Handle user input
		switch input {
		case "n":
			// Update request with next page token
			req.PageToken = resp.NextPageToken
		case "q":
			return nil
		default:
			if _, err := fmt.Fprintf(stream.Out, "Invalid input. Exiting pagination.\n"); err != nil {
				return err
			}
			return nil
		}
	}

	return nil
}

func printFormattedTr(s *cli.Stream, trs *v1.TaskRunList, c clockwork.Clock, allnamespaces bool, noheaders bool) error {
	var data = struct {
		TaskRuns      *v1.TaskRunList
		Time          clockwork.Clock
		AllNamespaces bool
		NoHeaders     bool
	}{
		TaskRuns:      trs,
		Time:          c,
		AllNamespaces: allnamespaces,
		NoHeaders:     noheaders,
	}

	funcMap := template.FuncMap{
		"formatAge":       common.FormatAge,
		"formatDuration":  formatted.Duration,
		"formatCondition": formatted.Condition,
	}

	w := tabwriter.NewWriter(s.Out, 0, 5, 3, ' ', tabwriter.TabIndent)
	t := template.Must(template.New("List TaskRuns").Funcs(funcMap).Parse(trListTemplate))

	err := t.Execute(w, data)
	if err != nil {
		return err
	}

	return w.Flush()
}

func parseRecordsToTr(records []*pb.Record) (*v1.TaskRunList, error) {
	var taskRuns = new(v1.TaskRunList)

	for _, record := range records {
		var tr v1.TaskRun
		if err := json.Unmarshal(record.Data.Value, &tr); err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON: %v", err)
		}
		taskRuns.Items = append(taskRuns.Items, tr)
	}

	return taskRuns, nil
}
