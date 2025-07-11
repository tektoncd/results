package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logtesting "knative.dev/pkg/logging/testing"
	"knative.dev/pkg/metrics/metricstest"
	_ "knative.dev/pkg/metrics/testing" // Required to set up metrics env for testing
)

var (
	nowTime        = metav1.Now()
	completionTime = metav1.NewTime(nowTime.Time.Add(-time.Minute))
)

func TestRecorder_RecordStorageLatency(t *testing.T) {
	tests := []struct {
		name                 string
		object               interface{}
		wantErr              bool
		expectedLatencyTags  map[string]string
		expectedLatencyValue float64
	}{
		{
			name: "successful PipelineRun storage",
			object: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: "pipelinerun-1", Namespace: "ns"},
				Spec: pipelinev1.PipelineRunSpec{
					PipelineRef: &pipelinev1.PipelineRef{Name: "pipeline-1"},
				},
				Status: pipelinev1.PipelineRunStatus{
					PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
						CompletionTime: &completionTime,
					},
				},
			},
			wantErr: false,
			expectedLatencyTags: map[string]string{
				"kind":      "pipelinerun",
				"namespace": "ns",
			},
			expectedLatencyValue: 60.0, // 1 minute
		},
		{
			name: "successful TaskRun storage",
			object: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{Name: "taskrun-1", Namespace: "ns"},
				Spec: pipelinev1.TaskRunSpec{
					TaskRef: &pipelinev1.TaskRef{Name: "task-1"},
				},
				Status: pipelinev1.TaskRunStatus{
					TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
						CompletionTime: &completionTime,
					},
				},
			},
			wantErr: false,
			expectedLatencyTags: map[string]string{
				"kind":      "taskrun",
				"namespace": "ns",
			},
			expectedLatencyValue: 60.0, // 1 minute
		},
		{
			name: "PipelineRun without completion time",
			object: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: "pipelinerun-3", Namespace: "ns"},
				Spec: pipelinev1.PipelineRunSpec{
					PipelineRef: &pipelinev1.PipelineRef{Name: "pipeline-1"},
				},
				Status: pipelinev1.PipelineRunStatus{
					PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
						// No CompletionTime set
					},
				},
			},
			wantErr: false,
			// Should not record any metrics since there's no completion time
		},
		{
			name: "TaskRun without completion time",
			object: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{Name: "taskrun-3", Namespace: "ns"},
				Spec: pipelinev1.TaskRunSpec{
					TaskRef: &pipelinev1.TaskRef{Name: "task-1"},
				},
				Status: pipelinev1.TaskRunStatus{
					TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
						// No CompletionTime set
					},
				},
			},
			wantErr: false,
			// Should not record any metrics since there's no completion time
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up fake clock
			fakeClock := clockwork.NewFakeClockAt(nowTime.Time)
			r := &Recorder{clock: fakeClock}

			// Set up metrics environment
			logger := logtesting.TestLogger(t)
			// Clean up any existing views first
			unregisterViews(logger)
			err := registerViews(logger)
			if err != nil {
				t.Fatalf("Failed to register view: %v", err)
			}

			// Record the metric
			err = r.RecordStorageLatency(context.Background(), tt.object)
			if (err != nil) != tt.wantErr {
				t.Errorf("RecordStorageLatency() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Check if metrics were recorded (only if completion time exists)
			if tt.expectedLatencyTags != nil {
				metricstest.CheckDistributionData(t, "run_storage_latency_seconds", tt.expectedLatencyTags, 1, tt.expectedLatencyValue, tt.expectedLatencyValue)
			} else {
				metricstest.CheckStatsNotReported(t, "run_storage_latency_seconds")
			}
		})
	}
}
