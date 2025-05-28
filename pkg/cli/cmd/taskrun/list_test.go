package taskrun

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"knative.dev/pkg/apis"

	"github.com/tektoncd/results/pkg/cli/flags"
	"github.com/tektoncd/results/pkg/cli/options"

	"github.com/jonboulle/clockwork"
	"github.com/spf13/cobra"
	"github.com/tektoncd/cli/pkg/cli"
	v1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tektoncd/results/pkg/cli/client"
	"github.com/tektoncd/results/pkg/cli/client/records"
	"github.com/tektoncd/results/pkg/cli/common"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

// testParams implements common.Params interface for testing
type taskRunListTestParams struct {
	common.ResultsParams
	Client       *client.RESTClient
	RecordClient records.RecordClient
}

type mockRecordClient struct {
	records.RecordClient
	listRecordsFunc func(ctx context.Context, req *pb.ListRecordsRequest, fields string) (*pb.ListRecordsResponse, error)
}

func (m *mockRecordClient) ListRecords(ctx context.Context, req *pb.ListRecordsRequest, fields string) (*pb.ListRecordsResponse, error) {
	return m.listRecordsFunc(ctx, req, fields)
}

// Mock implementation of GetRecordClient
func (p *taskRunListTestParams) GetRecordClient(_ context.Context) (records.RecordClient, error) {
	return p.RecordClient, nil
}

func TestListCommand(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		listRecords    func(ctx context.Context, in *pb.ListRecordsRequest, fields string) (*pb.ListRecordsResponse, error)
		expectedOutput string
		expectedError  bool
		expectedFilter string
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
								Value: []byte(`{"metadata":{"name":"taskrun-write-and-read-array-results-hjk57"},"status":{"conditions":[{"type":"Succeeded","status":"False"}]}}`),
							},
						},
						{
							Name: "test-record-2",
							Uid:  "test-uid-2",
							Data: &pb.Any{
								Value: []byte(`{"metadata":{"name":"test-taskrun-5np8f"},"status":{"conditions":[{"type":"Succeeded","status":"True"}]}}`),
							},
						},
					},
				}, nil
			},
			expectedOutput: "taskrun-write-and-read-array-results-hjk57",
			expectedError:  false,
		},
		{
			name: "list with task name filter",
			args: []string{"list", "test-task"},
			listRecords: func(_ context.Context, in *pb.ListRecordsRequest, _ string) (*pb.ListRecordsResponse, error) {
				expectedFilter := `(data_type=="tekton.dev/v1.TaskRun" || data_type=="tekton.dev/v1beta1.TaskRun") && data.metadata.name.contains("test-task")`
				if in.Filter != expectedFilter {
					t.Errorf("unexpected filter: got %v, want %v", in.Filter, expectedFilter)
				}
				return &pb.ListRecordsResponse{
					Records: []*pb.Record{
						{
							Name: "test-record",
							Uid:  "test-uid",
							Data: &pb.Any{
								Value: []byte(`{"metadata":{"name":"test-task-run-1"},"status":{"conditions":[{"type":"Succeeded","status":"True"}]}}`),
							},
						},
					},
				}, nil
			},
			expectedOutput: "test-task-run-1",
			expectedError:  false,
		},
		{
			name: "list with single label filter",
			args: []string{"list", "--label", "app=test"},
			listRecords: func(_ context.Context, in *pb.ListRecordsRequest, _ string) (*pb.ListRecordsResponse, error) {
				expectedFilter := `(data_type=="tekton.dev/v1.TaskRun" || data_type=="tekton.dev/v1beta1.TaskRun") && data.metadata.labels["app"]=="test"`
				if in.Filter != expectedFilter {
					t.Errorf("unexpected filter: got %v, want %v", in.Filter, expectedFilter)
				}
				return &pb.ListRecordsResponse{
					Records: []*pb.Record{
						{
							Name: "test-record",
							Uid:  "test-uid",
							Data: &pb.Any{
								Value: []byte(`{"metadata":{"name":"test-taskrun","labels":{"app":"test"}},"status":{"conditions":[{"type":"Succeeded","status":"True"}]}}`),
							},
						},
					},
				}, nil
			},
			expectedOutput: "test-taskrun",
			expectedError:  false,
		},
		{
			name: "list with multiple label filters",
			args: []string{"list", "--label", "app=test,env=prod"},
			listRecords: func(_ context.Context, in *pb.ListRecordsRequest, _ string) (*pb.ListRecordsResponse, error) {
				expectedFilter := `(data_type=="tekton.dev/v1.TaskRun" || data_type=="tekton.dev/v1beta1.TaskRun") && data.metadata.labels["app"]=="test" && data.metadata.labels["env"]=="prod"`
				if in.Filter != expectedFilter {
					t.Errorf("unexpected filter: got %v, want %v", in.Filter, expectedFilter)
				}
				return &pb.ListRecordsResponse{
					Records: []*pb.Record{
						{
							Name: "test-record",
							Uid:  "test-uid",
							Data: &pb.Any{
								Value: []byte(`{"metadata":{"name":"test-taskrun","labels":{"app":"test","env":"prod"}},"status":{"conditions":[{"type":"Succeeded","status":"True"}]}}`),
							},
						},
					},
				}, nil
			},
			expectedOutput: "test-taskrun",
			expectedError:  false,
		},
		{
			name: "list with invalid label format",
			args: []string{"list", "--label", "app=test,invalid"},
			listRecords: func(_ context.Context, _ *pb.ListRecordsRequest, _ string) (*pb.ListRecordsResponse, error) {
				return nil, nil
			},
			expectedOutput: "",
			expectedError:  true,
		},
		{
			name: "list with empty label value",
			args: []string{"list", "--label", "app="},
			listRecords: func(_ context.Context, _ *pb.ListRecordsRequest, _ string) (*pb.ListRecordsResponse, error) {
				return nil, nil
			},
			expectedOutput: "",
			expectedError:  true,
		},
		{
			name: "list with empty label key",
			args: []string{"list", "--label", "=test"},
			listRecords: func(_ context.Context, _ *pb.ListRecordsRequest, _ string) (*pb.ListRecordsResponse, error) {
				return nil, nil
			},
			expectedOutput: "",
			expectedError:  true,
		},
		{
			name: "list with task name and label filter",
			args: []string{"list", "test-task", "--label", "app=test"},
			listRecords: func(_ context.Context, in *pb.ListRecordsRequest, _ string) (*pb.ListRecordsResponse, error) {
				expectedFilter := `(data_type=="tekton.dev/v1.TaskRun" || data_type=="tekton.dev/v1beta1.TaskRun") && data.metadata.labels["app"]=="test" && data.metadata.name.contains("test-task")`
				if in.Filter != expectedFilter {
					t.Errorf("unexpected filter: got %v, want %v", in.Filter, expectedFilter)
				}
				return &pb.ListRecordsResponse{
					Records: []*pb.Record{
						{
							Name: "test-record",
							Uid:  "test-uid",
							Data: &pb.Any{
								Value: []byte(`{"metadata":{"name":"test-task-run-1","labels":{"app":"test"}},"status":{"conditions":[{"type":"Succeeded","status":"True"}]}}`),
							},
						},
					},
				}, nil
			},
			expectedOutput: "test-task-run-1",
			expectedError:  false,
		},
		{
			name: "list with pipelinerun filter",
			args: []string{"list", "--pipelinerun", "test-pipeline"},
			listRecords: func(_ context.Context, in *pb.ListRecordsRequest, _ string) (*pb.ListRecordsResponse, error) {
				expectedFilter := `(data_type=="tekton.dev/v1.TaskRun" || data_type=="tekton.dev/v1beta1.TaskRun") && data.metadata.labels['tekton.dev/pipelineRun'] == 'test-pipeline'`
				if in.Filter != expectedFilter {
					t.Errorf("unexpected filter: got %v, want %v", in.Filter, expectedFilter)
				}
				return &pb.ListRecordsResponse{
					Records: []*pb.Record{
						{
							Name: "test-record",
							Uid:  "test-uid",
							Data: &pb.Any{
								Value: []byte(`{"metadata":{"name":"test-taskrun","labels":{"tekton.dev/pipelineRun":"test-pipeline"}},"status":{"conditions":[{"type":"Succeeded","status":"True"}]}}`),
							},
						},
					},
				}, nil
			},
			expectedOutput: "test-taskrun",
			expectedError:  false,
		},
		{
			name: "list with pipelinerun and label filters",
			args: []string{"list", "--pipelinerun", "test-pipeline", "--label", "app=test"},
			listRecords: func(_ context.Context, in *pb.ListRecordsRequest, _ string) (*pb.ListRecordsResponse, error) {
				expectedFilter := `(data_type=="tekton.dev/v1.TaskRun" || data_type=="tekton.dev/v1beta1.TaskRun") && data.metadata.labels["app"]=="test" && data.metadata.labels['tekton.dev/pipelineRun'] == 'test-pipeline'`
				if in.Filter != expectedFilter {
					t.Errorf("unexpected filter: got %v, want %v", in.Filter, expectedFilter)
				}
				return &pb.ListRecordsResponse{
					Records: []*pb.Record{
						{
							Name: "test-record",
							Uid:  "test-uid",
							Data: &pb.Any{
								Value: []byte(`{"metadata":{"name":"test-taskrun","labels":{"app":"test","tekton.dev/pipelineRun":"test-pipeline"}},"status":{"conditions":[{"type":"Succeeded","status":"True"}]}}`),
							},
						},
					},
				}, nil
			},
			expectedOutput: "test-taskrun",
			expectedError:  false,
		},
		{
			name: "list with pipelinerun and name filters",
			args: []string{"list", "test-task", "--pipelinerun", "test-pipeline"},
			listRecords: func(_ context.Context, in *pb.ListRecordsRequest, _ string) (*pb.ListRecordsResponse, error) {
				expectedFilter := `(data_type=="tekton.dev/v1.TaskRun" || data_type=="tekton.dev/v1beta1.TaskRun") && data.metadata.name.contains("test-task") && data.metadata.labels['tekton.dev/pipelineRun'] == 'test-pipeline'`
				if in.Filter != expectedFilter {
					t.Errorf("unexpected filter: got %v, want %v", in.Filter, expectedFilter)
				}
				return &pb.ListRecordsResponse{
					Records: []*pb.Record{
						{
							Name: "test-record",
							Uid:  "test-uid",
							Data: &pb.Any{
								Value: []byte(`{"metadata":{"name":"test-task","labels":{"tekton.dev/pipelineRun":"test-pipeline"}},"status":{"conditions":[{"type":"Succeeded","status":"True"}]}}`),
							},
						},
					},
				}, nil
			},
			expectedOutput: "test-task",
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
			params := &taskRunListTestParams{
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
					ResourceType:  common.ResourceTypeTaskRun,
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
				if label, labelErr := cmd.Flags().GetString("label"); labelErr == nil && label != "" {
					opts.Label = label
					// Validate label format only if label is provided
					if err := common.ValidateLabels(label); err != nil {
						return err
					}
				}
				if pipelinerun, prErr := cmd.Flags().GetString("pipelinerun"); prErr == nil && pipelinerun != "" {
					opts.PipelineRun = pipelinerun
				}
				if len(tt.args) > 1 && tt.args[1] != "--label" && tt.args[1] != "--pipelinerun" {
					opts.ResourceName = tt.args[1]
				}

				// Build filter string
				filter := common.BuildFilterString(opts)

				// Handle all namespaces
				parent := fmt.Sprintf("%s/results/-", params.Namespace())
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

				// Use the mock client directly
				resp, listErr := mockClient.ListRecords(cmd.Context(), req, "")
				if listErr != nil {
					return listErr
				}

				stream := &cli.Stream{
					Out: cmd.OutOrStdout(),
					Err: cmd.OutOrStderr(),
				}

				// Parse records to TaskRuns before printing
				trs, err := parseRecordsToTr(resp.Records)
				if err != nil {
					return err
				}
				return printFormattedTr(stream, trs, clockwork.NewRealClock(), opts.AllNamespaces, false)
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

			// Verify the filter string
			if tt.expectedFilter != "" {
				opts := &options.ListOptions{
					PipelineRun:  cmd.Flag("pipelinerun").Value.String(),
					Label:        cmd.Flag("label").Value.String(),
					ResourceType: common.ResourceTypeTaskRun,
				}
				actualFilter := common.BuildFilterString(opts)
				if actualFilter != tt.expectedFilter {
					t.Errorf("Expected filter: %s, got: %s", tt.expectedFilter, actualFilter)
				}
			}
		})
	}
}

func createMockTaskRunRecords(namespace string, count int) []*pb.Record {
	records := make([]*pb.Record, count)
	for i := 0; i < count; i++ {
		tr := &v1.TaskRun{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "tekton.dev/v1",
				Kind:       "TaskRun",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("taskrun-%d", i),
				Namespace: namespace,
			},
			Status: v1.TaskRunStatus{
				Status: duckv1.Status{
					Conditions: []apis.Condition{
						{
							Type:   "Succeeded",
							Status: "True",
						},
					},
				},
				TaskRunStatusFields: v1.TaskRunStatusFields{
					StartTime:      &metav1.Time{Time: time.Now()},
					CompletionTime: &metav1.Time{Time: time.Now().Add(time.Hour)},
				},
			},
		}
		data, _ := json.Marshal(tr)
		records[i] = &pb.Record{
			Name: fmt.Sprintf("%s/results/-/records/taskrun-%d", namespace, i),
			Data: &pb.Any{
				Type:  "tekton.dev/v1.TaskRun",
				Value: data,
			},
		}
	}
	return records
}

func TestParseRecordsToTr(t *testing.T) {
	tests := []struct {
		name    string
		records []*pb.Record
		want    int
		wantErr bool
	}{
		{
			name:    "valid taskrun records",
			records: createMockTaskRunRecords("default", 2),
			want:    2,
		},
		{
			name: "invalid taskrun data",
			records: []*pb.Record{
				{
					Name: "test-record",
					Data: &pb.Any{
						Type:  "tekton.dev/v1.TaskRun",
						Value: []byte("invalid json"),
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRecordsToTr(tt.records)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseRecordsToTr() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got.Items) != tt.want {
				t.Errorf("parseRecordsToTr() got %d items, want %d", len(got.Items), tt.want)
			}
		})
	}
}

func TestBuildFilterString(t *testing.T) {
	tests := []struct {
		name           string
		opts           *options.ListOptions
		resourceType   string
		expectedFilter string
	}{
		{
			name: "pipelinerun filter only",
			opts: &options.ListOptions{
				PipelineRun:  "test-pipeline",
				ResourceType: common.ResourceTypeTaskRun,
			},
			resourceType:   common.ResourceTypeTaskRun,
			expectedFilter: `(data_type=="tekton.dev/v1.TaskRun" || data_type=="tekton.dev/v1beta1.TaskRun") && data.metadata.labels['tekton.dev/pipelineRun'] == 'test-pipeline'`,
		},
		{
			name: "pipelinerun and label filters",
			opts: &options.ListOptions{
				PipelineRun:  "test-pipeline",
				Label:        "app=test",
				ResourceType: common.ResourceTypeTaskRun,
			},
			resourceType:   common.ResourceTypeTaskRun,
			expectedFilter: `(data_type=="tekton.dev/v1.TaskRun" || data_type=="tekton.dev/v1beta1.TaskRun") && data.metadata.labels["app"]=="test" && data.metadata.labels['tekton.dev/pipelineRun'] == 'test-pipeline'`,
		},
		{
			name: "pipelinerun and name filters",
			opts: &options.ListOptions{
				PipelineRun:  "test-pipeline",
				ResourceName: "test-task",
				ResourceType: common.ResourceTypeTaskRun,
			},
			expectedFilter: `(data_type=="tekton.dev/v1.TaskRun" || data_type=="tekton.dev/v1beta1.TaskRun") && data.metadata.name.contains("test-task") && data.metadata.labels['tekton.dev/pipelineRun'] == 'test-pipeline'`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualFilter := common.BuildFilterString(tt.opts)
			if actualFilter != tt.expectedFilter {
				t.Errorf("Expected filter: %s, got: %s", tt.expectedFilter, actualFilter)
			}
		})
	}
}
