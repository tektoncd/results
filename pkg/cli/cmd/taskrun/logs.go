package taskrun

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

// NOTE:
// Logs are not supported for the system namespace and the default namespace for the LokiStack.

// logsCommand returns a cobra.Command that logs a TaskRun.
func logsCommand(p common.Params) *cobra.Command {
	opts := &options.LogsOptions{
		ResourceType: common.ResourceTypeTaskRun,
	}

	eg := `Get logs for a TaskRun named 'foo' in the current namespace:
  tkn-results taskrun logs foo

Get logs for a TaskRun in a specific namespace:
  tkn-results taskrun logs foo -n my-namespace

Get logs for a TaskRun by UID if there are multiple TaskRun with the same name:
  tkn-results taskrun logs --uid 12345678-1234-1234-1234-1234567890ab
`

	cmd := &cobra.Command{
		Use:   "logs [taskrun-name]",
		Short: "Get logs for a TaskRun",
		Long: `Get logs for a TaskRun by name or UID. If --uid is provided, the TaskRun name is optional.

If multiple TaskRuns match the given name, the logs for the most recent one are returned.
Use --uid to target a specific TaskRun when needed.

NOTE:
Logs are not supported for the system namespace or for the default namespace used by LokiStack.
Logs are only available for completed TaskRuns. Running TaskRuns do not have logs available yet.`,
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
				// Try direct primary key lookup first (works for standalone TaskRuns)
				r, err := recordClient.GetRecord(ctx, p.Namespace(), opts.UID)
				if err == nil {
					record = r
				} else {
					// Fallback: filter by record name column (text, indexed) instead
					// of data.metadata.uid (JSONB, unindexed). Needed for child
					// TaskRuns where the result UID is the parent PipelineRun UID.
					filter := fmt.Sprintf(`name=="%s"`, opts.UID)
					parent := fmt.Sprintf("%s/results/-", p.Namespace())
					resp, err := recordClient.ListRecords(ctx, &pb.ListRecordsRequest{
						Parent:   parent,
						Filter:   filter,
						OrderBy:  "create_time desc",
						PageSize: 5,
					}, common.NameUIDAndDataField)
					if err != nil {
						return fmt.Errorf("failed to find TaskRun: %v", err)
					}
					if len(resp.Records) == 0 {
						if opts.ResourceName != "" {
							return fmt.Errorf("no TaskRun found with name %s and UID %s", opts.ResourceName, opts.UID)
						}
						return fmt.Errorf("no TaskRun found with UID %s", opts.UID)
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
					return fmt.Errorf("failed to find TaskRun: %v", err)
				}
				if len(resp.Records) == 0 {
					return fmt.Errorf("no TaskRun found with name %s", opts.ResourceName)
				}
				record = resp.Records[0]
			}

			// Check if the TaskRun is completed before attempting to get logs
			var taskRun v1.TaskRun
			if err := json.Unmarshal(record.Data.Value, &taskRun); err != nil {
				return fmt.Errorf("failed to parse TaskRun data: %v", err)
			}

			if taskRun.Status.CompletionTime == nil {
				fmt.Println("Logs are not available for running TaskRuns. Please wait for the TaskRun to complete before retrieving logs.")
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
	cmd.Flags().StringVar(&opts.UID, "uid", "", "UID of the TaskRun to get logs for")

	return cmd
}
