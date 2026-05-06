package common_test

import (
	"testing"

	"github.com/tektoncd/results/pkg/cli/common"
	"github.com/tektoncd/results/pkg/cli/options"
)

func TestBuildFilterString(t *testing.T) {
	tests := []struct {
		name           string
		opts           common.FilterOptions
		expectedFilter string
	}{
		// TaskRun List Tests
		{
			name: "list: taskrun with pipelinerun filter only",
			opts: &options.ListOptions{
				PipelineRun:  "test-pipeline",
				ResourceType: common.ResourceTypeTaskRun,
			},
			expectedFilter: `(data_type=="tekton.dev/v1.TaskRun" || data_type=="tekton.dev/v1beta1.TaskRun") && data.metadata.labels['tekton.dev/pipelineRun'] == 'test-pipeline'`,
		},
		{
			name: "list: taskrun with pipelinerun and label filters",
			opts: &options.ListOptions{
				PipelineRun:  "test-pipeline",
				Label:        "app=test",
				ResourceType: common.ResourceTypeTaskRun,
			},
			expectedFilter: `(data_type=="tekton.dev/v1.TaskRun" || data_type=="tekton.dev/v1beta1.TaskRun") && data.metadata.labels["app"]=="test" && data.metadata.labels['tekton.dev/pipelineRun'] == 'test-pipeline'`,
		},
		{
			name: "list: taskrun with pipelinerun and name filters use contains",
			opts: &options.ListOptions{
				PipelineRun:  "test-pipeline",
				ResourceName: "test-task",
				ResourceType: common.ResourceTypeTaskRun,
			},
			expectedFilter: `(data_type=="tekton.dev/v1.TaskRun" || data_type=="tekton.dev/v1beta1.TaskRun") && data.metadata.name.contains("test-task") && data.metadata.labels['tekton.dev/pipelineRun'] == 'test-pipeline'`,
		},
		{
			name: "list: taskrun name filter uses contains",
			opts: &options.ListOptions{
				ResourceName: "my-taskrun",
				ResourceType: common.ResourceTypeTaskRun,
			},
			expectedFilter: `(data_type=="tekton.dev/v1.TaskRun" || data_type=="tekton.dev/v1beta1.TaskRun") && data.metadata.name.contains("my-taskrun")`,
		},
		// TaskRun Describe Tests
		{
			name: "describe: taskrun name filter uses exact match",
			opts: &options.DescribeOptions{
				ResourceName: "my-taskrun",
				ResourceType: common.ResourceTypeTaskRun,
			},
			expectedFilter: `(data_type=="tekton.dev/v1.TaskRun" || data_type=="tekton.dev/v1beta1.TaskRun") && data.metadata.name=="my-taskrun"`,
		},
		{
			name: "describe: taskrun name and UID filters use exact match",
			opts: &options.DescribeOptions{
				ResourceName: "my-taskrun",
				UID:          "abc-123",
				ResourceType: common.ResourceTypeTaskRun,
			},
			expectedFilter: `(data_type=="tekton.dev/v1.TaskRun" || data_type=="tekton.dev/v1beta1.TaskRun") && data.metadata.name=="my-taskrun" && data.metadata.uid=="abc-123"`,
		},
		{
			name: "describe: taskrun UID-only filter uses exact match",
			opts: &options.DescribeOptions{
				UID:          "abc-123",
				ResourceType: common.ResourceTypeTaskRun,
			},
			expectedFilter: `(data_type=="tekton.dev/v1.TaskRun" || data_type=="tekton.dev/v1beta1.TaskRun") && data.metadata.uid=="abc-123"`,
		},
		// PipelineRun List Tests
		{
			name: "list: pipelinerun name filter uses contains",
			opts: &options.ListOptions{
				ResourceName: "my-pipeline",
				ResourceType: common.ResourceTypePipelineRun,
			},
			expectedFilter: `(data_type=="tekton.dev/v1.PipelineRun" || data_type=="tekton.dev/v1beta1.PipelineRun") && data.metadata.name.contains("my-pipeline")`,
		},
		{
			name: "list: pipelinerun label filter only",
			opts: &options.ListOptions{
				Label:        "app=test",
				ResourceType: common.ResourceTypePipelineRun,
			},
			expectedFilter: `(data_type=="tekton.dev/v1.PipelineRun" || data_type=="tekton.dev/v1beta1.PipelineRun") && data.metadata.labels["app"]=="test"`,
		},
		{
			name: "list: pipelinerun name and label filters use contains",
			opts: &options.ListOptions{
				ResourceName: "my-pipeline",
				Label:        "app=test",
				ResourceType: common.ResourceTypePipelineRun,
			},
			expectedFilter: `(data_type=="tekton.dev/v1.PipelineRun" || data_type=="tekton.dev/v1beta1.PipelineRun") && data.metadata.labels["app"]=="test" && data.metadata.name.contains("my-pipeline")`,
		},
		// PipelineRun Describe Tests
		{
			name: "describe: pipelinerun name filter uses exact match",
			opts: &options.DescribeOptions{
				ResourceName: "my-pipeline",
				ResourceType: common.ResourceTypePipelineRun,
			},
			expectedFilter: `(data_type=="tekton.dev/v1.PipelineRun" || data_type=="tekton.dev/v1beta1.PipelineRun") && data.metadata.name=="my-pipeline"`,
		},
		{
			name: "describe: pipelinerun name and UID filters use exact match",
			opts: &options.DescribeOptions{
				ResourceName: "my-pipeline",
				UID:          "abc-123",
				ResourceType: common.ResourceTypePipelineRun,
			},
			expectedFilter: `(data_type=="tekton.dev/v1.PipelineRun" || data_type=="tekton.dev/v1beta1.PipelineRun") && data.metadata.name=="my-pipeline" && data.metadata.uid=="abc-123"`,
		},
		{
			name: "describe: pipelinerun UID-only filter uses exact match",
			opts: &options.DescribeOptions{
				UID:          "abc-123",
				ResourceType: common.ResourceTypePipelineRun,
			},
			expectedFilter: `(data_type=="tekton.dev/v1.PipelineRun" || data_type=="tekton.dev/v1beta1.PipelineRun") && data.metadata.uid=="abc-123"`,
		},
		// Logs Tests
		{
			name: "logs: taskrun name filter uses exact match",
			opts: &options.LogsOptions{
				ResourceName: "my-taskrun",
				ResourceType: common.ResourceTypeTaskRun,
			},
			expectedFilter: `(data_type=="tekton.dev/v1.TaskRun" || data_type=="tekton.dev/v1beta1.TaskRun") && data.metadata.name=="my-taskrun"`,
		},
		{
			name: "logs: pipelinerun name filter uses exact match",
			opts: &options.LogsOptions{
				ResourceName: "my-pipeline",
				ResourceType: common.ResourceTypePipelineRun,
			},
			expectedFilter: `(data_type=="tekton.dev/v1.PipelineRun" || data_type=="tekton.dev/v1beta1.PipelineRun") && data.metadata.name=="my-pipeline"`,
		},
		{
			name: "logs: UID-only filter uses exact match",
			opts: &options.LogsOptions{
				UID:          "abc-123",
				ResourceType: common.ResourceTypePipelineRun,
			},
			expectedFilter: `(data_type=="tekton.dev/v1.PipelineRun" || data_type=="tekton.dev/v1beta1.PipelineRun") && data.metadata.uid=="abc-123"`,
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
