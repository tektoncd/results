package taskrun

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/tektoncd/results/pkg/cli/client/logs"
	"github.com/tektoncd/results/pkg/cli/client/records"
	"github.com/tektoncd/results/pkg/cli/common"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockLogsClient struct {
	logs.Client
	getLogFunc func(context.Context, *pb.GetLogRequest) (io.Reader, error)
}

func (m *mockLogsClient) GetLog(ctx context.Context, req *pb.GetLogRequest) (io.Reader, error) {
	return m.getLogFunc(ctx, req)
}

type mockRecordsClient struct {
	records.RecordClient
	listRecordsFunc func(context.Context, *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error)
}

func (m *mockRecordsClient) ListRecords(ctx context.Context, req *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error) {
	return m.listRecordsFunc(ctx, req)
}

func TestLogsCommand(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		mockListFunc   func(context.Context, *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error)
		mockGetLogFunc func(context.Context, *pb.GetLogRequest) (io.Reader, error)
		wantErr        bool
		wantOutput     string
	}{
		{
			name: "successful log retrieval by name",
			args: []string{"test-taskrun"},
			mockListFunc: func(_ context.Context, _ *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error) {
				return &pb.ListRecordsResponse{
					Records: []*pb.Record{
						{
							Name: "test-record",
							Uid:  "test-uid",
						},
					},
				}, nil
			},
			mockGetLogFunc: func(_ context.Context, _ *pb.GetLogRequest) (io.Reader, error) {
				return strings.NewReader("test log content"), nil
			},
			wantErr:    false,
			wantOutput: "test log content",
		},
		{
			name: "successful log retrieval by UID",
			args: []string{"--uid", "test-uid"},
			mockListFunc: func(_ context.Context, _ *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error) {
				return &pb.ListRecordsResponse{
					Records: []*pb.Record{
						{
							Name: "test-record",
							Uid:  "test-uid",
						},
					},
				}, nil
			},
			mockGetLogFunc: func(_ context.Context, _ *pb.GetLogRequest) (io.Reader, error) {
				return strings.NewReader("test log content"), nil
			},
			wantErr:    false,
			wantOutput: "test log content",
		},
		{
			name: "no TaskRun found",
			args: []string{"non-existent"},
			mockListFunc: func(_ context.Context, _ *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error) {
				return &pb.ListRecordsResponse{
					Records: []*pb.Record{},
				}, nil
			},
			mockGetLogFunc: nil,
			wantErr:        true,
			wantOutput:     "no TaskRun found with name non-existent",
		},
		{
			name: "multiple TaskRuns found",
			args: []string{"ambiguous-name"},
			mockListFunc: func(_ context.Context, _ *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error) {
				return &pb.ListRecordsResponse{
					Records: []*pb.Record{
						{
							Name: "record-1",
							Uid:  "uid-1",
						},
						{
							Name: "record-2",
							Uid:  "uid-2",
						},
					},
				}, nil
			},
			mockGetLogFunc: nil,
			wantErr:        true,
			wantOutput:     "multiple TaskRuns found",
		},
		{
			name: "error getting logs",
			args: []string{"test-taskrun"},
			mockListFunc: func(_ context.Context, _ *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error) {
				return &pb.ListRecordsResponse{
					Records: []*pb.Record{
						{
							Name: "test-record",
							Uid:  "test-uid",
						},
					},
				}, nil
			},
			mockGetLogFunc: func(_ context.Context, _ *pb.GetLogRequest) (io.Reader, error) {
				return nil, status.Error(codes.Internal, "failed to get logs")
			},
			wantErr:    true,
			wantOutput: "failed to get logs",
		},
		{
			name:       "invalid arguments",
			args:       []string{},
			wantErr:    true,
			wantOutput: "requires exactly one argument when --uid is not provided",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new command with mock clients
			params := &common.ResultsParams{}
			params.SetNamespace("test-namespace")
			cmd := logsCommand(params)

			// Set up mock clients
			cmd.PersistentPreRunE = nil // Disable the default PreRunE
			cmd.PreRunE = func(_ *cobra.Command, _ []string) error {
				cmd.SetContext(context.Background())
				return nil
			}

			// Override the RunE function to use our mock clients
			cmd.RunE = func(cmd *cobra.Command, args []string) error {
				if tt.mockListFunc == nil {
					return fmt.Errorf("requires exactly one argument when --uid is not provided")
				}

				ctx := cmd.Context()
				recordClient := &mockRecordsClient{
					listRecordsFunc: tt.mockListFunc,
				}
				logsClient := &mockLogsClient{
					getLogFunc: tt.mockGetLogFunc,
				}

				// Build filter string
				filter := []string{`data_type==TASK_RUN`}
				if len(args) > 0 {
					filter = append(filter, `data.metadata.name.contains("`+args[0]+`")`)
				}

				// Find the TaskRun
				resp, err := recordClient.ListRecords(ctx, &pb.ListRecordsRequest{
					Parent:   "test-namespace/results/-",
					Filter:   strings.Join(filter, " && "),
					PageSize: 10,
				})
				if err != nil {
					return err
				}

				if len(resp.Records) == 0 {
					return errors.New("no TaskRun found with name " + args[0])
				}

				if len(resp.Records) > 1 {
					return errors.New("multiple TaskRuns found")
				}

				// Get the logs
				reader, err := logsClient.GetLog(ctx, &pb.GetLogRequest{
					Name: resp.Records[0].Name,
				})
				if err != nil {
					return err
				}

				// Read the logs directly to the command's output
				_, err = io.Copy(cmd.OutOrStdout(), reader)
				return err
			}

			// Execute the command
			cmd.SetArgs(tt.args)
			var out bytes.Buffer
			cmd.SetOut(&out)
			err := cmd.Execute()

			// Check the error
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if !strings.Contains(err.Error(), tt.wantOutput) {
					t.Errorf("error message %q does not contain %q", err.Error(), tt.wantOutput)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				// Check the output
				if got := out.String(); got != tt.wantOutput {
					t.Errorf("got output %q, want %q", got, tt.wantOutput)
				}
			}
		})
	}
}
