package pipelinerun

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	v1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tektoncd/results/pkg/cli/options"

	"github.com/spf13/cobra"
	"github.com/tektoncd/results/pkg/cli/client/logs"
	"github.com/tektoncd/results/pkg/cli/client/records"
	"github.com/tektoncd/results/pkg/cli/common"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

// logsCommand returns a cobra.Command that logs a PipelineRun.
func logsCommand(p common.Params) *cobra.Command {
	opts := &options.LogsOptions{
		ResourceType: common.ResourceTypePipelineRun,
	}

	eg := `Get logs for a PipelineRun named 'foo' in the current namespace:
  tkn-results pipelinerun logs foo

Get logs for a PipelineRun in a specific namespace:
  tkn-results pipelinerun logs foo -n my-namespace

Get logs for a PipelineRun by UID if there are multiple PipelineRuns with the same name:
  tkn-results pipelinerun logs --uid 12345678-1234-1234-1234-1234567890ab
`

	cmd := &cobra.Command{
		Use:   "logs [pipelinerun-name]",
		Short: "Get logs for a PipelineRun",
		Long: `Get logs for a PipelineRun by name or UID. If --uid is provided, the PipelineRun name is optional.

If multiple PipelineRuns match the given name, the logs for the most recent one are returned.
Use --uid to target a specific PipelineRun when needed.

NOTE:
Logs are not supported for the system namespace or for the default namespace used by LokiStack.
Additionally, PipelineRun logs are not supported for S3 log storage.
Logs are only available for completed PipelineRuns. Running PipelineRuns do not have logs available yet.`,
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
					filter := fmt.Sprintf(`name.endsWith("records/%s")`, opts.UID)
					parent := fmt.Sprintf("%s/results/-", p.Namespace())
					resp, err := recordClient.ListRecords(ctx, &pb.ListRecordsRequest{
						Parent:   parent,
						Filter:   filter,
						OrderBy:  "create_time desc",
						PageSize: 5,
					}, common.NameUIDAndDataField)
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
				}, common.NameUIDAndDataField)
				if err != nil {
					return fmt.Errorf("failed to find PipelineRun: %v", err)
				}
				if len(resp.Records) == 0 {
					return fmt.Errorf("no PipelineRun found with name %s", opts.ResourceName)
				}
				record = resp.Records[0]
			}

			// Check if the PipelineRun is completed before attempting to get logs
			var pipelineRun v1.PipelineRun
			if err := json.Unmarshal(record.Data.Value, &pipelineRun); err != nil {
				return fmt.Errorf("failed to parse PipelineRun data: %v", err)
			}

			if pipelineRun.Status.CompletionTime == nil {
				fmt.Println("Logs are not available for running PipelineRuns. Please wait for the PipelineRun to complete before retrieving logs.")
				return nil
			}

			// Create a new logs client
			lc := logs.NewClient(opts.Client)

			// Create a request to get the logs
			req := &pb.GetLogRequest{
				Name: record.Name,
			}

			// Get the logs
			reader, err := lc.GetLog(ctx, req)
			if err != nil {
				return fmt.Errorf("failed to get logs: %v", err)
			}

			// Close the reader if it implements io.Closer
			if closer, ok := reader.(io.Closer); ok {
				// Workaround for golangci-lint returned value not checked complaint
				defer func() {
					_ = closer.Close()
				}()
			}

			// Copy the logs to stdout
			if _, err := io.Copy(os.Stdout, reader); err != nil {
				return fmt.Errorf("failed to copy logs: %v", err)
			}

			return nil
		},
	}
	cmd.Flags().StringVar(&opts.UID, "uid", "", "UID of the PipelineRun to get logs for")

	return cmd
}
