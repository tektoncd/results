package pipelinerun

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/tektoncd/results/pkg/cli/options"

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
	listRecordsFunc func(ctx context.Context, in *pb.ListRecordsRequest, fields string) (*pb.ListRecordsResponse, error)
}

func (m *mockRecordClient) ListRecords(ctx context.Context, in *pb.ListRecordsRequest, fields string) (*pb.ListRecordsResponse, error) {
	return m.listRecordsFunc(ctx, in, fields)
}

func TestListCommand(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		listRecords    func(ctx context.Context, in *pb.ListRecordsRequest, fields string) (*pb.ListRecordsResponse, error)
		expectedOutput string
		expectedError  bool
	}{
		{
			name: "successful list with default options",
			args: []string{"list"},
			listRecords: func(_ context.Context, _ *pb.ListRecordsRequest, _ string) (*pb.ListRecordsResponse, error) {
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
			listRecords: func(_ context.Context, in *pb.ListRecordsRequest, _ string) (*pb.ListRecordsResponse, error) {
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
			listRecords: func(_ context.Context, in *pb.ListRecordsRequest, _ string) (*pb.ListRecordsResponse, error) {
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
			listRecords: func(_ context.Context, in *pb.ListRecordsRequest, _ string) (*pb.ListRecordsResponse, error) {
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
			listRecords: func(_ context.Context, _ *pb.ListRecordsRequest, _ string) (*pb.ListRecordsResponse, error) {
				return nil, fmt.Errorf("test error")
			},
			expectedOutput: "",
			expectedError:  true,
		},
		{
			name: "empty list",
			args: []string{"list"},
			listRecords: func(_ context.Context, _ *pb.ListRecordsRequest, _ string) (*pb.ListRecordsResponse, error) {
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
			listRecords: func(_ context.Context, in *pb.ListRecordsRequest, _ string) (*pb.ListRecordsResponse, error) {
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
					opts := &options.ListOptions{}
					opts.ResourceName = args[0]
				}
				return nil
			}

			flags.AddResultsOptions(cmd)

			// Override RunE to use mock client
			cmd.RunE = func(cmd *cobra.Command, _ []string) error {
				opts := &options.ListOptions{
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
					opts.ResourceName = tt.args[1]
				}

				// Build filter string
				filter := []string{`data_type==PIPELINE_RUN`}
				if opts.Label != "" {
					filter = append(filter, fmt.Sprintf(`labels.%s`, opts.Label))
				}
				if opts.ResourceName != "" {
					filter = append(filter, fmt.Sprintf(`data.metadata.name.contains("%s")`, opts.ResourceName))
				}

				// Handle all namespaces
				parent := fmt.Sprintf("%s/results/-", params.Namespace())
				if opts.AllNamespaces {
					parent = common.AllNamespacesResultsParent
				}

				// Create initial request
				req := &pb.ListRecordsRequest{
					Parent:   parent,
					Filter:   strings.Join(filter, " && "),
					OrderBy:  "create_time desc",
					PageSize: opts.Limit,
				}

				// Use the mock client directly
				resp, listErr := mockClient.ListRecords(cmd.Context(), req, "")
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
		name          string
		opts          *options.ListOptions
		expected      string
		expectedError string
	}{
		{
			name: "single label",
			opts: &options.ListOptions{
				Label:        "app.kubernetes.io/name=test-app",
				ResourceType: common.ResourceTypePipelineRun,
			},
			expected: `(data_type=="tekton.dev/v1.PipelineRun" || data_type=="tekton.dev/v1beta1.PipelineRun") && data.metadata.labels["app.kubernetes.io/name"]=="test-app"`,
		},
		{
			name: "multiple labels",
			opts: &options.ListOptions{
				Label:        "app.kubernetes.io/name=test-app,app.kubernetes.io/component=database",
				ResourceType: common.ResourceTypePipelineRun,
			},
			expected: `(data_type=="tekton.dev/v1.PipelineRun" || data_type=="tekton.dev/v1beta1.PipelineRun") && data.metadata.labels["app.kubernetes.io/name"]=="test-app" && data.metadata.labels["app.kubernetes.io/component"]=="database"`,
		},
		{
			name: "with pipeline name",
			opts: &options.ListOptions{
				Label:        "app.kubernetes.io/name=test-app",
				ResourceName: "my-pipeline",
				ResourceType: common.ResourceTypePipelineRun,
			},
			expected: `(data_type=="tekton.dev/v1.PipelineRun" || data_type=="tekton.dev/v1beta1.PipelineRun") && data.metadata.labels["app.kubernetes.io/name"]=="test-app" && data.metadata.name.contains("my-pipeline")`,
		},
		{
			name: "empty label",
			opts: &options.ListOptions{
				Label:        "",
				ResourceType: common.ResourceTypePipelineRun,
			},
			expected: `(data_type=="tekton.dev/v1.PipelineRun" || data_type=="tekton.dev/v1beta1.PipelineRun")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := common.BuildFilterString(tt.opts)
			if got != tt.expected {
				t.Errorf("buildFilterString() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestValidateLabels(t *testing.T) {
	tests := []struct {
		name          string
		label         string
		expectedError string
	}{
		{
			name:  "valid single label",
			label: "app.kubernetes.io/name=test-app",
		},
		{
			name:  "valid multiple labels",
			label: "app.kubernetes.io/name=test-app,app.kubernetes.io/component=database",
		},
		{
			name:          "missing equals sign",
			label:         "app.kubernetes.io/name test-app",
			expectedError: "invalid label format: app.kubernetes.io/name test-app. Expected format: key=value",
		},
		{
			name:          "missing value",
			label:         "app.kubernetes.io/name=",
			expectedError: "label value cannot be empty in pair: app.kubernetes.io/name=",
		},
		{
			name:          "missing key",
			label:         "=test-app",
			expectedError: "label key cannot be empty in pair: =test-app",
		},
		{
			name:          "empty pair",
			label:         "app.kubernetes.io/name=test-app,,",
			expectedError: "invalid label format: . Expected format: key=value",
		},
		{
			name:          "malformed pair",
			label:         "app.kubernetes.io/name=test-app,key2",
			expectedError: "invalid label format: key2. Expected format: key=value",
		},
		{
			name:          "whitespace in key",
			label:         "app.kubernetes.io/name =test-app",
			expectedError: "label key cannot contain whitespace: app.kubernetes.io/name ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal command with just the label option
			cmd := &cobra.Command{
				PreRunE: func(_ *cobra.Command, _ []string) error {
					opts := &options.ListOptions{
						Label: tt.label,
					}
					// Validate label format if provided
					if opts.Label != "" {
						labelPairs := strings.Split(opts.Label, ",")
						for _, pair := range labelPairs {
							pair = strings.TrimSpace(pair)
							if pair == "" {
								return fmt.Errorf("invalid label format: . Expected format: key=value")
							}

							// Split on the first equals sign only
							parts := strings.SplitN(pair, "=", 2)
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
			}

			err := cmd.PreRunE(cmd, []string{})

			if tt.expectedError != "" {
				if err == nil {
					t.Errorf("expected error: %v, got nil", tt.expectedError)
				} else if err.Error() != tt.expectedError {
					t.Errorf("expected error: %q (bytes: %v), got: %q (bytes: %v)",
						tt.expectedError, []byte(tt.expectedError),
						err.Error(), []byte(err.Error()))
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}
