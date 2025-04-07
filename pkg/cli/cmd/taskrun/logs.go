package taskrun

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tektoncd/results/pkg/cli/options"

	"github.com/spf13/cobra"
	"github.com/tektoncd/results/pkg/cli/client"
	"github.com/tektoncd/results/pkg/cli/client/logs"
	"github.com/tektoncd/results/pkg/cli/client/records"
	"github.com/tektoncd/results/pkg/cli/common"
	"github.com/tektoncd/results/pkg/cli/config"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

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

Get logs for a TaskRun from all namespaces:
  tkn-results taskrun logs foo -A
`

	cmd := &cobra.Command{
		Use:   "logs [taskrun-name]",
		Short: "Get logs for a TaskRun",
		Long:  "Get logs for a TaskRun by name or UID. If --uid is provided, the TaskRun name is optional.",
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
		PreRunE: func(cmd *cobra.Command, _ []string) error {
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
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if len(args) > 0 {
				opts.ResourceName = args[0]
			}

			// Build filter string to find the TaskRun
			filter := common.BuildFilterString(opts)

			// Handle namespace
			parent := fmt.Sprintf("%s/results/-", p.Namespace())
			if opts.AllNamespaces {
				parent = "*/results/-"
			}

			// Create record client
			recordClient := records.NewClient(opts.Client)

			// Find the TaskRun record
			resp, err := recordClient.ListRecords(ctx, &pb.ListRecordsRequest{
				Parent:   parent,
				Filter:   filter,
				PageSize: 25,
			}, common.NameAndUIDField)
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

			// If multiple TaskRuns are found, return an error
			if len(resp.Records) > 1 {
				var uids []string
				for _, record := range resp.Records {
					uids = append(uids, record.Uid)
				}
				return fmt.Errorf("multiple TaskRuns found. Use a more specific name or UID. Available UIDs: %s",
					strings.Join(uids, ", "))
			}

			// Get the TaskRun record
			record := resp.Records[0]

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
				defer closer.Close()
			}

			// Copy the logs to stdout
			if _, err := io.Copy(os.Stdout, reader); err != nil {
				return fmt.Errorf("failed to copy logs: %v", err)
			}

			return nil
		},
	}
	cmd.Flags().BoolVarP(&opts.AllNamespaces, "all-namespaces", "A", false, "use all namespaces")
	cmd.Flags().StringVar(&opts.UID, "uid", "", "UID of the TaskRun to get logs for")

	return cmd
}
