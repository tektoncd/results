package pipelinerunmetrics

import (
	"context"

	"github.com/jonboulle/clockwork"
	pipelinev1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/results/pkg/apis/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
	logtesting "knative.dev/pkg/logging/testing"
	_ "knative.dev/pkg/metrics/testing" // Required to set up metrics env for testing

	"testing"
	"time"

	"knative.dev/pkg/metrics/metricstest"
)

var (
	nowTime        = metav1.Now()
	completionTime = metav1.NewTime(nowTime.Time.Add(-time.Minute))
	failedTime     = metav1.NewTime(nowTime.Time.Add(-time.Second * 30))
	startTime      = metav1.NewTime(nowTime.Time.Add(-time.Minute * 2))
)

func TestRecorder_DurationAndCountDeleted(t *testing.T) {
	tests := []struct {
		name                 string
		pr                   *pipelinev1beta1.PipelineRun
		wantErr              bool
		expectedDurationTags map[string]string
		expectedCountTags    map[string]string
		expectedDuration     float64
		expectedCount        int64
	}{
		{
			name: "for succeed pipeline",
			pr: &pipelinev1beta1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: "pipelinerun-1", Namespace: "ns"},
				Spec: pipelinev1beta1.PipelineRunSpec{
					PipelineRef: &pipelinev1beta1.PipelineRef{Name: "pipeline-1"},
				},
				Status: pipelinev1beta1.PipelineRunStatus{
					Status: duckv1beta1.Status{
						Conditions: duckv1beta1.Conditions{{
							Type:   apis.ConditionSucceeded,
							Status: corev1.ConditionTrue,
						}},
					},
					PipelineRunStatusFields: pipelinev1beta1.PipelineRunStatusFields{
						StartTime:      &startTime,
						CompletionTime: &completionTime,
					},
				},
			},
			wantErr: false,
			expectedDurationTags: map[string]string{
				"pipeline":  "pipeline-1",
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
			name: "for canceled pipeline (without completion time)",
			pr: &pipelinev1beta1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: "pipelinerun-1", Namespace: "ns"},
				Spec: pipelinev1beta1.PipelineRunSpec{
					PipelineRef: &pipelinev1beta1.PipelineRef{Name: "pipeline-1"},
				},
				Status: pipelinev1beta1.PipelineRunStatus{
					Status: duckv1beta1.Status{
						Conditions: duckv1beta1.Conditions{{
							Type:               apis.ConditionSucceeded,
							Status:             corev1.ConditionFalse,
							Reason:             "Cancelled",
							LastTransitionTime: apis.VolatileTime{Inner: failedTime},
						}},
					},
					PipelineRunStatusFields: pipelinev1beta1.PipelineRunStatusFields{
						StartTime: &startTime,
					},
				},
			},
			wantErr: false,
			expectedDurationTags: map[string]string{
				"pipeline":  "pipeline-1",
				"namespace": "ns",
				"status":    "cancelled",
			},
			expectedCountTags: map[string]string{
				"namespace": "ns",
				"status":    "cancelled",
			},
			expectedDuration: 30,
			expectedCount:    1,
		},
		{
			name: "for failed pipeline (without completion time)",
			pr: &pipelinev1beta1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: "pipelinerun-1", Namespace: "ns"},
				Spec: pipelinev1beta1.PipelineRunSpec{
					PipelineRef: &pipelinev1beta1.PipelineRef{Name: "pipeline-1"},
				},
				Status: pipelinev1beta1.PipelineRunStatus{
					Status: duckv1beta1.Status{
						Conditions: duckv1beta1.Conditions{{
							Type:               apis.ConditionSucceeded,
							Status:             corev1.ConditionFalse,
							LastTransitionTime: apis.VolatileTime{Inner: failedTime},
						}},
					},
					PipelineRunStatusFields: pipelinev1beta1.PipelineRunStatusFields{
						StartTime: &startTime,
					},
				},
			},
			wantErr: false,
			expectedDurationTags: map[string]string{
				"pipeline":  "pipeline-1",
				"namespace": "ns",
				"status":    "failed",
			},
			expectedCountTags: map[string]string{
				"namespace": "ns",
				"status":    "failed",
			},
			expectedDuration: 30,
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

			if err := r.DurationAndCountDeleted(context.Background(), cfg, tt.pr); (err != nil) != tt.wantErr {
				t.Errorf("DurationAndCountDeleted() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.expectedDurationTags != nil {
				metricstest.CheckLastValueData(t, "pipelinerun_delete_duration_seconds", tt.expectedDurationTags, tt.expectedDuration)
			} else {
				metricstest.CheckStatsNotReported(t, "pipelinerun_delete_duration_seconds")
			}
			if tt.expectedCountTags != nil {
				metricstest.CheckCountData(t, "pipelinerun_delete_count", tt.expectedCountTags, tt.expectedCount)
			} else {
				metricstest.CheckStatsNotReported(t, "pipelinerun_delete_count")
			}
		})
	}
}
