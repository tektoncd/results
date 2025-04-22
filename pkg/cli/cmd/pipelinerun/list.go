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

type listOptions struct {
	Client        *client.RESTClient
	Limit         int32
	AllNamespaces bool
	Label         string
	SinglePage    bool
	PipelineName  string
}

// listCommand initializes a cobra command to list PipelineRuns
func listCommand(p common.Params) *cobra.Command {
	opts := &listOptions{
		Limit:         50,
		AllNamespaces: false,
		SinglePage:    true,
	}

	eg := `List all PipelineRuns in a namespace 'foo':
    tkn-results pipelinerun list -n foo

List all PipelineRuns in 'default' namespace:
    tkn-results pipelinerun list -n default

List all PipelineRuns using the pagination, not the single page
    tkn-results pipelinerun list --single-page false

List PipelineRuns with a specific label:
    tkn-results pipelinerun list -l app=myapp

List PipelineRuns with multiple label selectors:
    tkn-results pipelinerun list -l app=myapp,env=prod

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
		PreRunE: func(_ *cobra.Command, args []string) error {
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
				opts.PipelineName = args[0]
			}

			// Validate label format if provided
			if opts.Label != "" {
				labelPairs := strings.Split(opts.Label, ",")
				for _, pair := range labelPairs {
					parts := strings.Split(strings.TrimSpace(pair), "=")
					if len(parts) != 2 {
						return fmt.Errorf("invalid label format: %s. Expected format: key=value", pair)
					}

					// Check for whitespace in key before trimming
					if strings.ContainsAny(parts[0], " \t") {
						return fmt.Errorf("label key cannot contain whitespace: %s", parts[0])
					}

					key := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])

					if key == "" {
						return fmt.Errorf("label key cannot be empty in pair: %s", pair)
					}
					if value == "" {
						return fmt.Errorf("label value cannot be empty in pair: %s", pair)
					}
				}
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Build filter string
			filter := buildFilterString(opts)

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

			// If single page is requested, just fetch and print
			if opts.SinglePage {
				resp, err := recordClient.ListRecords(cmd.Context(), req)
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
				return printFormattedPr(stream, prs, clockwork.NewRealClock(), opts.AllNamespaces, false)
			}

			// Interactive pagination
			reader := bufio.NewReader(os.Stdin)
			for {
				resp, err := recordClient.ListRecords(cmd.Context(), req)
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

				// If there's no next page token, we're done
				if resp.NextPageToken == "" {
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

	cmd.Flags().Int32VarP(&opts.Limit, "limit", "l", 50, "Maximum number of PipelineRuns to return (must be between 5 and 1000 and defaults to 50)")
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
		"formatAge":       formatted.Age,
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

// buildFilterString constructs the filter string for the ListRecordsRequest
func buildFilterString(opts *listOptions) string {
	const (
		contains = "data.metadata.%s.contains(\"%s\")"
		equal    = "data.metadata.%s[\"%s\"]==\"%s\""
		dataType = "data_type==\"%s\""
	)

	var filters []string

	// Add data type filter for both v1 and v1beta1 PipelineRuns
	filters = append(filters, fmt.Sprintf(`(%s || %s)`,
		fmt.Sprintf(dataType, "tekton.dev/v1.PipelineRun"),
		fmt.Sprintf(dataType, "tekton.dev/v1beta1.PipelineRun")))

	// Handle label filters
	if opts.Label != "" {
		// Split by comma to get individual label pairs
		labelPairs := strings.Split(opts.Label, ",")
		for _, pair := range labelPairs {
			// Split each pair by = to get key and value
			parts := strings.Split(strings.TrimSpace(pair), "=")
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				filters = append(filters, fmt.Sprintf(equal, "labels", key, value))
			}
		}
	}

	// Handle pipeline name filter
	if opts.PipelineName != "" {
		filters = append(filters, fmt.Sprintf(contains, "name", opts.PipelineName))
	}
	return strings.Join(filters, " && ")
}
