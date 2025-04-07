package pipelinerun

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"text/template"

	"github.com/tektoncd/results/pkg/cli/options"

	"github.com/tektoncd/results/pkg/cli/client"
	"github.com/tektoncd/results/pkg/cli/client/records"

	"github.com/jonboulle/clockwork"
	"github.com/spf13/cobra"
	"github.com/tektoncd/cli/pkg/cli"
	"github.com/tektoncd/cli/pkg/formatted"
	v1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tektoncd/results/pkg/cli/common"
	"github.com/tektoncd/results/pkg/cli/config"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

const prListTemplate = `{{- $prl := len .PipelineRuns.Items -}}{{- if eq $prl 0 -}}
No PipelineRuns found
{{ else -}}
{{- if not $.NoHeaders -}}
{{- if $.AllNamespaces -}}
NAMESPACE	NAME	UID	STARTED	DURATION	STATUS
{{ else -}}
NAME	UID	STARTED	DURATION	STATUS
{{ end -}}
{{- end -}}
{{- range $_, $pr := .PipelineRuns.Items }}{{- if $pr }}{{- if $.AllNamespaces -}}
{{ $pr.Namespace }}	{{ $pr.Name }} {{ $pr.UID }}	{{ formatAge $pr.Status.StartTime $.Time }}	{{ formatDuration $pr.Status.StartTime $pr.Status.CompletionTime }}	{{ formatCondition $pr.Status.Conditions }}
{{ else -}}
{{ $pr.Name }}	{{ $pr.UID }}	{{ formatAge $pr.Status.StartTime $.Time }}	{{ formatDuration $pr.Status.StartTime $pr.Status.CompletionTime }}	{{ formatCondition $pr.Status.Conditions }}
{{ end -}}{{- end -}}{{- end -}}
{{- end -}}`

// listCommand initializes a cobra command to list PipelineRuns
func listCommand(p common.Params) *cobra.Command {
	opts := &options.ListOptions{
		Limit:         50,
		AllNamespaces: false,
		SinglePage:    true,
		ResourceType:  common.ResourceTypePipelineRun,
	}

	eg := `List all PipelineRuns in a namespace 'foo':
    tkn-results pipelinerun list -n foo

List PipelineRuns with a specific label:
    tkn-results pipelinerun list -L app=myapp

List PipelineRuns with multiple label selectors:
    tkn-results pipelinerun list -L app=myapp,env=prod

List PipelineRuns from all namespaces:
    tkn-results pipelinerun list -A

List PipelineRuns with limit of 20 per page:
    tkn-results pipelinerun list --limit=20

List PipelineRuns for a specific pipeline:
    tkn-results pipelinerun list foo -n namespace

List PipelineRuns with partial pipeline name match:
    tkn-results pipelinerun list build -n namespace
`
	cmd := &cobra.Command{
		Use:     "list [pipeline-name]",
		Aliases: []string{"ls"},
		Short:   "List PipelineRuns in a namespace",
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
			c, err := config.NewConfig(p)
			if err != nil {
				return err
			}
			opts.Client, err = client.NewRESTClient(c.Get())
			if err != nil {
				return err
			}

			if opts.Limit < 5 || opts.Limit > 1000 {
				return errors.New("limit should be between 5 and 1000")
			}

			if len(args) > 0 {
				opts.ResourceName = args[0]
			}

			// Validate label format if provided
			if opts.Label != "" {
				return common.ValidateLabels(opts.Label)
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Build filter string
			filter := common.BuildFilterString(opts)

			// Handle all namespaces
			parent := fmt.Sprintf("%s/results/-", p.Namespace())
			if opts.AllNamespaces {
				parent = "*/results/-"
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
				resp, err := recordClient.ListRecords(cmd.Context(), req, common.ListFields)
				if err != nil {
					return err
				}

				stream := &cli.Stream{
					Out: cmd.OutOrStdout(),
					Err: cmd.OutOrStderr(),
				}

				// Parse records to PipelineRuns before printing
				prs, err := parseRecordsToPr(resp.Records)
				if err != nil {
					return err
				}
				if err := printFormattedPr(stream, prs, clockwork.NewRealClock(), opts.AllNamespaces, false); err != nil {
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
		},
	}

	cmd.Flags().Int32VarP(&opts.Limit, "limit", "", 50, "Maximum number of PipelineRuns to return (must be between 5 and 1000 and defaults to 50)")
	cmd.Flags().BoolVarP(&opts.AllNamespaces, "all-namespaces", "A", false, "List PipelineRuns from all namespaces")
	cmd.Flags().StringVarP(&opts.Label, "label", "L", "", "Filter by label (format: key=value,key2=value2)")
	cmd.Flags().BoolVar(&opts.SinglePage, "single-page", true, "Return only a single page of results")

	return cmd
}

func printFormattedPr(s *cli.Stream, prs *v1.PipelineRunList, c clockwork.Clock, allnamespaces bool, noheaders bool) error {
	var data = struct {
		PipelineRuns  *v1.PipelineRunList
		Time          clockwork.Clock
		AllNamespaces bool
		NoHeaders     bool
	}{
		PipelineRuns:  prs,
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
	t := template.Must(template.New("List PipelineRuns").Funcs(funcMap).Parse(prListTemplate))

	err := t.Execute(w, data)
	if err != nil {
		return err
	}

	return w.Flush()
}

func parseRecordsToPr(records []*pb.Record) (*v1.PipelineRunList, error) {
	var pipelineRuns = new(v1.PipelineRunList)
	for _, record := range records {
		var pr v1.PipelineRun
		if err := json.Unmarshal(record.Data.Value, &pr); err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
		}
		pipelineRuns.Items = append(pipelineRuns.Items, pr)
	}
	return pipelineRuns, nil
}
