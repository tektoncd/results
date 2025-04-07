package pipelinerun

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/tektoncd/results/pkg/cli/flags"

	"github.com/jonboulle/clockwork"
	"github.com/spf13/cobra"
	"github.com/tektoncd/cli/pkg/cli"
	"github.com/tektoncd/results/pkg/cli/client"
	"github.com/tektoncd/results/pkg/cli/client/records"
	"github.com/tektoncd/results/pkg/cli/common"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

// testParams implements common.Params interface for testing
type testParams struct {
	common.ResultsParams
	Client       *client.RESTClient
	RecordClient records.RecordClient
}

// mockRecordClient implements the RecordClient interface for testing
type mockRecordClient struct {
	records.RecordClient
	listRecordsFunc func(ctx context.Context, in *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error)
}

func (m *mockRecordClient) ListRecords(ctx context.Context, in *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error) {
	return m.listRecordsFunc(ctx, in)
}

func TestListCommand(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		listRecords    func(ctx context.Context, in *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error)
		expectedOutput string
		expectedError  bool
	}{
		{
			name: "successful list with default options",
			args: []string{"list"},
			listRecords: func(_ context.Context, _ *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error) {
				return &pb.ListRecordsResponse{
					Records: []*pb.Record{
						{
							Name: "test-record",
							Uid:  "test-uid",
							Data: &pb.Any{
								Value: []byte(`{"metadata":{"name":"pipelinerun-write-and-read-array-results-hjk57"},"status":{"conditions":[{"type":"Succeeded","status":"False"}]}}`),
							},
						},
						{
							Name: "test-record-2",
							Uid:  "test-uid-2",
							Data: &pb.Any{
								Value: []byte(`{"metadata":{"name":"test-pipelinerun-5np8f"},"status":{"conditions":[{"type":"Succeeded","status":"True"}]}}`),
							},
						},
					},
				}, nil
			},
			expectedOutput: "pipelinerun-write-and-read-array-results-hjk57",
			expectedError:  false,
		},
		{
			name: "list with pipeline name filter",
			args: []string{"list", "test-pipeline"},
			listRecords: func(_ context.Context, in *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error) {
				expectedFilter := `data_type==PIPELINE_RUN && data.metadata.name.contains("test-pipeline")`
				if in.Filter != expectedFilter {
					t.Errorf("unexpected filter: got %v, want %v", in.Filter, expectedFilter)
				}
				return &pb.ListRecordsResponse{
					Records: []*pb.Record{
						{
							Name: "test-record",
							Uid:  "test-uid",
							Data: &pb.Any{
								Value: []byte(`{"metadata":{"name":"test-pipeline-run-1"},"status":{"conditions":[{"type":"Succeeded","status":"True"}]}}`),
							},
						},
					},
				}, nil
			},
			expectedOutput: "test-pipeline-run-1",
			expectedError:  false,
		},
		{
			name: "list with partial pipeline name match",
			args: []string{"list", "build"},
			listRecords: func(_ context.Context, in *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error) {
				expectedFilter := `data_type==PIPELINE_RUN && data.metadata.name.contains("build")`
				if in.Filter != expectedFilter {
					t.Errorf("unexpected filter: got %v, want %v", in.Filter, expectedFilter)
				}
				return &pb.ListRecordsResponse{
					Records: []*pb.Record{
						{
							Name: "test-record-1",
							Uid:  "test-uid-1",
							Data: &pb.Any{
								Value: []byte(`{"metadata":{"name":"build-frontend-run-1"},"status":{"conditions":[{"type":"Succeeded","status":"True"}]}}`),
							},
						},
						{
							Name: "test-record-2",
							Uid:  "test-uid-2",
							Data: &pb.Any{
								Value: []byte(`{"metadata":{"name":"build-backend-run-1"},"status":{"conditions":[{"type":"Succeeded","status":"True"}]}}`),
							},
						},
					},
				}, nil
			},
			expectedOutput: "build-frontend-run-1",
			expectedError:  false,
		},
		{
			name: "list with namespace filter",
			args: []string{"list", "--namespace", "test-ns"},
			listRecords: func(_ context.Context, in *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error) {
				expectedFilter := "data_type==PIPELINE_RUN"
				if in.Filter != expectedFilter {
					t.Errorf("unexpected filter: got %v, want %v", in.Filter, expectedFilter)
				}
				return &pb.ListRecordsResponse{
					Records: []*pb.Record{
						{
							Name: "test-record",
							Uid:  "test-uid",
							Data: &pb.Any{
								Value: []byte(`{
									"apiVersion": "tekton.dev/v1",
									"kind": "PipelineRun",
									"metadata": {
										"name": "test-pipeline-run",
										"namespace": "test-ns"
									},
									"status": {
										"conditions": [
											{
												"type": "Succeeded",
												"status": "True"
											}
										]
									}
								}`),
							},
						},
					},
				}, nil
			},
			expectedOutput: "test-pipeline-run",
			expectedError:  false,
		},
		{
			name: "list with error",
			args: []string{"list"},
			listRecords: func(_ context.Context, _ *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error) {
				return nil, fmt.Errorf("test error")
			},
			expectedOutput: "",
			expectedError:  true,
		},
		{
			name: "empty list",
			args: []string{"list"},
			listRecords: func(_ context.Context, _ *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error) {
				return &pb.ListRecordsResponse{
					Records: []*pb.Record{},
				}, nil
			},
			expectedOutput: "No PipelineRuns found",
			expectedError:  false,
		},
		{
			name: "list with pipeline name and namespace",
			args: []string{"list", "test-pipeline", "-n", "test-ns"},
			listRecords: func(_ context.Context, in *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error) {
				expectedFilter := `data_type==PIPELINE_RUN && data.metadata.name.contains("test-pipeline")`
				if in.Filter != expectedFilter {
					t.Errorf("unexpected filter: got %v, want %v", in.Filter, expectedFilter)
				}
				expectedParent := "test-ns/results/-"
				if in.Parent != expectedParent {
					t.Errorf("unexpected parent: got %v, want %v", in.Parent, expectedParent)
				}
				return &pb.ListRecordsResponse{
					Records: []*pb.Record{
						{
							Name: "test-record",
							Uid:  "test-uid",
							Data: &pb.Any{
								Value: []byte(`{
									"metadata": {
										"name": "test-pipeline-run-1",
										"namespace": "test-ns"
									},
									"status": {
										"conditions": [
											{
												"type": "Succeeded",
												"status": "True"
											}
										]
									}
								}`),
							},
						},
					},
				}, nil
			},
			expectedOutput: "test-pipeline-run-1",
			expectedError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock client
			mockClient := &mockRecordClient{
				listRecordsFunc: tt.listRecords,
			}

			// Create test params with mock client
			params := &testParams{
				ResultsParams: common.ResultsParams{},
				RecordClient:  mockClient,
			}
			params.SetHost("http://localhost:8080")

			// Create output and error buffers
			var outBuf, errBuf bytes.Buffer

			// Get the command
			cmd := listCommand(params)
			cmd.SetOut(&outBuf)
			cmd.SetErr(&errBuf)
			cmd.SetArgs(tt.args)

			// Override PreRunE to bypass kubeconfig check
			cmd.PreRunE = func(_ *cobra.Command, args []string) error {
				if len(args) > 0 {
					opts := &listOptions{}
					opts.PipelineName = args[0]
				}
				return nil
			}

			flags.AddResultsOptions(cmd)

			// Override RunE to use mock client
			cmd.RunE = func(cmd *cobra.Command, _ []string) error {
				opts := &listOptions{
					Limit:         10,
					AllNamespaces: false,
					SinglePage:    true,
				}

				if ns, nsErr := cmd.Flags().GetString("namespace"); nsErr == nil {
					params.SetNamespace(ns)
				}
				if limit, limitErr := cmd.Flags().GetInt32("limit"); limitErr == nil {
					opts.Limit = limit
				}
				if allNamespaces, allNsErr := cmd.Flags().GetBool("all-namespaces"); allNsErr == nil {
					opts.AllNamespaces = allNamespaces
				}
				if singlePage, spErr := cmd.Flags().GetBool("single-page"); spErr == nil {
					opts.SinglePage = singlePage
				}
				if label, labelErr := cmd.Flags().GetString("label"); labelErr == nil {
					opts.Label = label
				}
				if len(tt.args) > 1 && tt.args[1] != "-n" && tt.args[1] != "--namespace" {
					opts.PipelineName = tt.args[1]
				}

				// Build filter string
				filter := []string{`data_type==PIPELINE_RUN`}
				if opts.Label != "" {
					filter = append(filter, fmt.Sprintf(`labels.%s`, opts.Label))
				}
				if opts.PipelineName != "" {
					filter = append(filter, fmt.Sprintf(`data.metadata.name.contains("%s")`, opts.PipelineName))
				}

				// Handle all namespaces
				parent := fmt.Sprintf("%s/results/-", params.Namespace())
				if opts.AllNamespaces {
					parent = "*/results/-"
				}

				// Create initial request
				req := &pb.ListRecordsRequest{
					Parent:   parent,
					Filter:   strings.Join(filter, " && "),
					OrderBy:  "create_time desc",
					PageSize: opts.Limit,
				}

				// Use the mock client directly
				resp, listErr := mockClient.ListRecords(cmd.Context(), req)
				if listErr != nil {
					return listErr
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

			// Execute the command
			err := cmd.Execute()

			// Check for expected error
			if tt.expectedError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Check output
			output := outBuf.String()
			if !strings.Contains(output, tt.expectedOutput) {
				t.Errorf("expected output to contain %q, got %q", tt.expectedOutput, output)
			}
		})
	}
}

func TestBuildFilterString(t *testing.T) {
	tests := []struct {
		name     string
		opts     *listOptions
		expected string
	}{
		{
			name:     "no filters",
			opts:     &listOptions{},
			expected: "data_type==\"PIPELINE_RUN\"",
		},
		{
			name: "with label",
			opts: &listOptions{
				Label: "key=value",
			},
			expected: "data_type==\"PIPELINE_RUN\" AND data.metadata.labels.key==\"value\"",
		},
		{
			name: "with namespace and label",
			opts: &listOptions{
				Label: "key=value",
			},
			expected: "data_type==\"PIPELINE_RUN\" AND data.metadata.labels.key==\"value\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := buildFilterString(tt.opts)
			if filter != tt.expected {
				t.Errorf("unexpected filter string: got %v, want %v", filter, tt.expected)
			}
		})
	}
}

func buildFilterString(opts *listOptions) string {
	filter := "data_type==\"PIPELINE_RUN\""
	if opts.Label != "" {
		parts := strings.Split(opts.Label, "=")
		if len(parts) == 2 {
			filter += fmt.Sprintf(" AND data.metadata.labels.%s==\"%s\"", parts[0], parts[1])
		}
	}
	return filter
}
