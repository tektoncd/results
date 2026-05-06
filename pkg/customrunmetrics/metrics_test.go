/*
Copyright 2026 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package customrunmetrics

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	pipelinev1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/results/pkg/apis/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

var (
	nowTime        = metav1.Now()
	completionTime = metav1.NewTime(nowTime.Add(-time.Minute))
	startTime      = metav1.NewTime(nowTime.Add(-time.Minute * 2))
)

func TestRecorder_DurationAndCountDeleted(t *testing.T) {
	tests := []struct {
		name                 string
		customRun            *pipelinev1beta1.CustomRun
		wantErr              bool
		expectedDurationTags map[string]string
		expectedCountTags    map[string]string
		expectedDuration     float64
		expectedCount        int64
	}{
		{
			name: "for succeeded customrun",
			customRun: &pipelinev1beta1.CustomRun{
				ObjectMeta: metav1.ObjectMeta{Name: "customrun-1", Namespace: "ns"},
				Spec: pipelinev1beta1.CustomRunSpec{
					CustomRef: &pipelinev1beta1.TaskRef{Name: "custom-task-1"},
				},
				Status: pipelinev1beta1.CustomRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{{
							Type:   apis.ConditionSucceeded,
							Status: corev1.ConditionTrue,
						}},
					},
					CustomRunStatusFields: pipelinev1beta1.CustomRunStatusFields{
						StartTime:      &startTime,
						CompletionTime: &completionTime,
					},
				},
			},
			expectedDurationTags: map[string]string{
				"customrun": "custom-task-1",
				"pipeline":  "anonymous",
				"namespace": "ns",
				"status":    "success",
			},
			expectedCountTags: map[string]string{
				"customrun": "custom-task-1",
				"pipeline":  "anonymous",
				"namespace": "ns",
				"status":    "success",
			},
			expectedDuration: 60,
			expectedCount:    1,
		},
		{
			name: "for failed customrun",
			customRun: &pipelinev1beta1.CustomRun{
				ObjectMeta: metav1.ObjectMeta{Name: "customrun-1", Namespace: "ns"},
				Spec: pipelinev1beta1.CustomRunSpec{
					CustomRef: &pipelinev1beta1.TaskRef{Name: "custom-task-1"},
				},
				Status: pipelinev1beta1.CustomRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{{
							Type:   apis.ConditionSucceeded,
							Status: corev1.ConditionFalse,
						}},
					},
					CustomRunStatusFields: pipelinev1beta1.CustomRunStatusFields{
						StartTime:      &startTime,
						CompletionTime: &completionTime,
					},
				},
			},
			expectedDurationTags: map[string]string{
				"customrun": "custom-task-1",
				"pipeline":  "anonymous",
				"namespace": "ns",
				"status":    "failed",
			},
			expectedCountTags: map[string]string{
				"customrun": "custom-task-1",
				"pipeline":  "anonymous",
				"namespace": "ns",
				"status":    "failed",
			},
			expectedDuration: 60,
			expectedCount:    1,
		},
		{
			name: "for succeeded customrun in pipelinerun",
			customRun: &pipelinev1beta1.CustomRun{
				ObjectMeta: metav1.ObjectMeta{
					Name: "customrun-1", Namespace: "ns",
					Labels: map[string]string{
						pipeline.PipelineLabelKey:    "pipeline-1",
						pipeline.PipelineRunLabelKey: "pipelinerun-1",
					},
				},
				Spec: pipelinev1beta1.CustomRunSpec{
					CustomRef: &pipelinev1beta1.TaskRef{Name: "custom-task-1"},
				},
				Status: pipelinev1beta1.CustomRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{{
							Type:   apis.ConditionSucceeded,
							Status: corev1.ConditionTrue,
						}},
					},
					CustomRunStatusFields: pipelinev1beta1.CustomRunStatusFields{
						StartTime:      &startTime,
						CompletionTime: &completionTime,
					},
				},
			},
			expectedDurationTags: map[string]string{
				"pipeline":  "pipeline-1",
				"customrun": "custom-task-1",
				"namespace": "ns",
				"status":    "success",
			},
			expectedCountTags: map[string]string{
				"pipeline":  "pipeline-1",
				"customrun": "custom-task-1",
				"namespace": "ns",
				"status":    "success",
			},
			expectedDuration: 60,
			expectedCount:    1,
		}, {
			name: "for cancelled customrun in pipelinerun",
			customRun: &pipelinev1beta1.CustomRun{
				ObjectMeta: metav1.ObjectMeta{
					Name: "customrun-1", Namespace: "ns",
					Labels: map[string]string{
						pipeline.PipelineLabelKey:    "pipeline-1",
						pipeline.PipelineRunLabelKey: "pipelinerun-1",
					},
				},
				Spec: pipelinev1beta1.CustomRunSpec{
					CustomRef: &pipelinev1beta1.TaskRef{Name: "custom-task-1"},
				},
				Status: pipelinev1beta1.CustomRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{{
							Type:   apis.ConditionSucceeded,
							Status: corev1.ConditionFalse,
							Reason: pipelinev1beta1.CustomRunReasonCancelled.String(),
						}},
					},
					CustomRunStatusFields: pipelinev1beta1.CustomRunStatusFields{
						StartTime:      &startTime,
						CompletionTime: &completionTime,
					},
				},
			},
			expectedDurationTags: map[string]string{
				"pipeline":  "pipeline-1",
				"customrun": "custom-task-1",
				"namespace": "ns",
				"status":    "cancelled",
			},
			expectedCountTags: map[string]string{
				"pipeline":  "pipeline-1",
				"customrun": "custom-task-1",
				"namespace": "ns",
				"status":    "cancelled",
			},
			expectedDuration: 60,
			expectedCount:    1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up OpenTelemetry metric reader
			reader := metric.NewManualReader()
			provider := metric.NewMeterProvider(metric.WithReader(reader))
			otel.SetMeterProvider(provider)
			defer otel.SetMeterProvider(nil)

			// Initialize metrics
			logger := zaptest.NewLogger(t).Sugar()
			once = sync.Once{} // Reset once for testing
			if err := initializeMetrics(logger); err != nil {
				t.Fatalf("Failed to initialize metrics: %v", err)
			}

			r := &Recorder{
				clock: clockwork.NewFakeClockAt(nowTime.Time),
			}

			cfg := &config.Metrics{
				TaskrunLevel:            config.DefaultTaskrunLevel,
				PipelinerunLevel:        config.DefaultPipelinerunLevel,
				DurationTaskrunType:     config.DurationTaskrunTypeHistogram,
				DurationPipelinerunType: config.DurationPipelinerunTypeHistogram,
			}

			if err := r.DurationAndCountDeleted(context.Background(), cfg, tt.customRun); (err != nil) != tt.wantErr {
				t.Errorf("DurationAndCountDeleted() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Collect metrics
			rm := &metricdata.ResourceMetrics{}
			if err := reader.Collect(context.Background(), rm); err != nil {
				t.Fatalf("Failed to collect metrics: %v", err)
			}

			// Check metrics
			if tt.expectedDurationTags != nil {
				checkHistogramData(t, rm, "watcher_customrun_delete_duration_seconds", tt.expectedDurationTags, 1, tt.expectedDuration)
			} else {
				checkMetricNotReported(t, rm, "watcher_customrun_delete_duration_seconds")
			}
			if tt.expectedCountTags != nil {
				checkSumData(t, rm, "watcher_customrun_delete_count", tt.expectedCountTags, tt.expectedCount)
			} else {
				checkMetricNotReported(t, rm, "watcher_customrun_delete_count")
			}
		})
	}
}

// Helper functions for testing OpenTelemetry metrics

func checkHistogramData(t *testing.T, rm *metricdata.ResourceMetrics, name string, expectedTags map[string]string, expectedCount uint64, expectedSum float64) {
	t.Helper()

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				histogram, ok := m.Data.(metricdata.Histogram[float64])
				if !ok {
					t.Errorf("Expected histogram data for %s, got %T", name, m.Data)
					return
				}

				for _, dp := range histogram.DataPoints {
					if attributesMatch(dp.Attributes, expectedTags) {
						if dp.Count != expectedCount {
							t.Errorf("Expected count %d for %s, got %d", expectedCount, name, dp.Count)
						}
						if dp.Sum != expectedSum {
							t.Errorf("Expected sum %f for %s, got %f", expectedSum, name, dp.Sum)
						}
						return
					}
				}
			}
		}
	}
	t.Errorf("Metric %s with tags %v not found", name, expectedTags)
}

func checkSumData(t *testing.T, rm *metricdata.ResourceMetrics, name string, expectedTags map[string]string, expectedValue int64) {
	t.Helper()

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				sum, ok := m.Data.(metricdata.Sum[int64])
				if !ok {
					t.Errorf("Expected sum data for %s, got %T", name, m.Data)
					return
				}

				for _, dp := range sum.DataPoints {
					if attributesMatch(dp.Attributes, expectedTags) {
						if dp.Value != expectedValue {
							t.Errorf("Expected value %d for %s, got %d", expectedValue, name, dp.Value)
						}
						return
					}
				}
			}
		}
	}
	t.Errorf("Metric %s with tags %v not found", name, expectedTags)
}

func attributesMatch(attrs attribute.Set, expected map[string]string) bool {
	if len(expected) != attrs.Len() {
		return false
	}
	for k, v := range expected {
		val, ok := attrs.Value(attribute.Key(k))
		if !ok || val.AsString() != v {
			return false
		}
	}
	return true
}

func checkMetricNotReported(t *testing.T, rm *metricdata.ResourceMetrics, name string) {
	t.Helper()

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				t.Errorf("Metric %s should not be reported", name)
			}
		}
	}
}
