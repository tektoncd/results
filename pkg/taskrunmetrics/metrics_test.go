package taskrunmetrics

import (
	"context"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	pipelinev1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/results/pkg/apis/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
	logtesting "knative.dev/pkg/logging/testing"
	"knative.dev/pkg/metrics/metricstest"
	_ "knative.dev/pkg/metrics/testing" // Required to set up metrics env for testing
)

var (
	nowTime        = metav1.Now()
	completionTime = metav1.NewTime(nowTime.Time.Add(-time.Minute))
	startTime      = metav1.NewTime(nowTime.Time.Add(-time.Minute * 2))
)

func TestRecorder_DurationAndCountDeleted(t *testing.T) {
	tests := []struct {
		name                 string
		taskRun              *pipelinev1beta1.TaskRun
		wantErr              bool
		expectedDurationTags map[string]string
		expectedCountTags    map[string]string
		expectedDuration     float64
		expectedCount        int64
	}{
		{
			name: "for succeeded taskrun",
			taskRun: &pipelinev1beta1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{Name: "taskrun-1", Namespace: "ns"},
				Spec: pipelinev1beta1.TaskRunSpec{
					TaskRef: &pipelinev1beta1.TaskRef{Name: "task-1"},
				},
				Status: pipelinev1beta1.TaskRunStatus{
					Status: duckv1beta1.Status{
						Conditions: duckv1beta1.Conditions{{
							Type:   apis.ConditionSucceeded,
							Status: corev1.ConditionTrue,
						}},
					},
					TaskRunStatusFields: pipelinev1beta1.TaskRunStatusFields{
						StartTime:      &startTime,
						CompletionTime: &completionTime,
					},
				},
			},
			expectedDurationTags: map[string]string{
				"task":      "task-1",
				"pipeline":  "anonymous",
				"namespace": "ns",
				"status":    "success",
			},
			expectedCountTags: map[string]string{
				"namespace": "ns",
				"status":    "success",
			},
			expectedDuration: 60,
			expectedCount:    1,
		},
		{
			name: "for failed taskrun",
			taskRun: &pipelinev1beta1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{Name: "taskrun-1", Namespace: "ns"},
				Spec: pipelinev1beta1.TaskRunSpec{
					TaskRef: &pipelinev1beta1.TaskRef{Name: "task-1"},
				},
				Status: pipelinev1beta1.TaskRunStatus{
					Status: duckv1beta1.Status{
						Conditions: duckv1beta1.Conditions{{
							Type:   apis.ConditionSucceeded,
							Status: corev1.ConditionFalse,
						}},
					},
					TaskRunStatusFields: pipelinev1beta1.TaskRunStatusFields{
						StartTime:      &startTime,
						CompletionTime: &completionTime,
					},
				},
			},
			expectedDurationTags: map[string]string{
				"task":      "task-1",
				"pipeline":  "anonymous",
				"namespace": "ns",
				"status":    "failed",
			},
			expectedCountTags: map[string]string{
				"namespace": "ns",
				"status":    "failed",
			},
			expectedDuration: 60,
			expectedCount:    1,
		},
		{
			name: "for succeeded taskrun in pipelinerun",
			taskRun: &pipelinev1beta1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name: "taskrun-1", Namespace: "ns",
					Labels: map[string]string{
						pipeline.PipelineLabelKey:    "pipeline-1",
						pipeline.PipelineRunLabelKey: "pipelinerun-1",
					},
				},
				Spec: pipelinev1beta1.TaskRunSpec{
					TaskRef: &pipelinev1beta1.TaskRef{Name: "task-1"},
				},
				Status: pipelinev1beta1.TaskRunStatus{
					Status: duckv1beta1.Status{
						Conditions: duckv1beta1.Conditions{{
							Type:   apis.ConditionSucceeded,
							Status: corev1.ConditionTrue,
						}},
					},
					TaskRunStatusFields: pipelinev1beta1.TaskRunStatusFields{
						StartTime:      &startTime,
						CompletionTime: &completionTime,
					},
				},
			},
			expectedDurationTags: map[string]string{
				"pipeline":  "pipeline-1",
				"task":      "task-1",
				"namespace": "ns",
				"status":    "success",
			},
			expectedCountTags: map[string]string{
				"namespace": "ns",
				"status":    "success",
			},
			expectedDuration: 60,
			expectedCount:    1,
		}, {
			name: "for failed taskrun in pipelinerun",
			taskRun: &pipelinev1beta1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name: "taskrun-1", Namespace: "ns",
					Labels: map[string]string{
						pipeline.PipelineLabelKey:    "pipeline-1",
						pipeline.PipelineRunLabelKey: "pipelinerun-1",
					},
				},
				Spec: pipelinev1beta1.TaskRunSpec{
					TaskRef: &pipelinev1beta1.TaskRef{Name: "task-1"},
				},
				Status: pipelinev1beta1.TaskRunStatus{
					Status: duckv1beta1.Status{
						Conditions: duckv1beta1.Conditions{{
							Type:   apis.ConditionSucceeded,
							Status: corev1.ConditionFalse,
						}},
					},
					TaskRunStatusFields: pipelinev1beta1.TaskRunStatusFields{
						StartTime:      &startTime,
						CompletionTime: &completionTime,
					},
				},
			},
			expectedDurationTags: map[string]string{
				"pipeline":  "pipeline-1",
				"task":      "task-1",
				"namespace": "ns",
				"status":    "failed",
			},
			expectedCountTags: map[string]string{
				"namespace": "ns",
				"status":    "failed",
			},
			expectedDuration: 60,
			expectedCount:    1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Recorder{
				clock: clockwork.NewFakeClockAt(nowTime.Time),
			}

			cfg := &config.Metrics{
				TaskrunLevel:            config.DefaultTaskrunLevel,
				PipelinerunLevel:        config.DefaultPipelinerunLevel,
				DurationTaskrunType:     config.DurationTaskrunTypeLastValue,
				DurationPipelinerunType: config.DurationPipelinerunTypeLastValue,
			}

			logger := logtesting.TestLogger(t)
			viewUnregister(logger)
			_ = viewRegister(logger, cfg)

			if err := r.DurationAndCountDeleted(context.Background(), cfg, tt.taskRun); (err != nil) != tt.wantErr {
				t.Errorf("DurationAndCountDeleted() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.expectedDurationTags != nil {
				metricstest.CheckLastValueData(t, "taskrun_delete_duration_seconds", tt.expectedDurationTags, tt.expectedDuration)
			} else {
				metricstest.CheckStatsNotReported(t, "taskrun_delete_duration_seconds")
			}
			if tt.expectedCountTags != nil {
				metricstest.CheckCountData(t, "taskrun_delete_count", tt.expectedCountTags, tt.expectedCount)
			} else {
				metricstest.CheckStatsNotReported(t, "taskrun_delete_count")
			}
		})
	}
}
