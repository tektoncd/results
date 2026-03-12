// Package metrics provides unified metrics recording for Tekton Results.
// It includes metrics for tracking runs not stored and storage latency for both PipelineRuns and TaskRuns.
package metrics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

var (
	once sync.Once

	// OpenTelemetry instruments
	runsNotStoredCount metric.Int64Counter
	runStorageLatency  metric.Float64Histogram

	// Common attribute keys
	namespaceKey = attribute.Key("namespace")
	kindKey      = attribute.Key("kind")
)

// Recorder is used to record metrics for both PipelineRuns and TaskRuns
type Recorder struct {
	clock clockwork.Clock
}

// NewRecorder creates a new metrics recorder instance
func NewRecorder() *Recorder {
	return &Recorder{clock: clockwork.NewRealClock()}
}

func initializeMetrics(logger *zap.SugaredLogger) error {
	meter := otel.Meter("tekton.dev/results")

	var err error

	// Create runs not stored counter
	runsNotStoredCount, err = meter.Int64Counter(
		"runs_not_stored_count",
		metric.WithDescription("total number of runs which were deleted without being stored"),
	)
	if err != nil {
		return fmt.Errorf("failed to create runs_not_stored_count counter: %w", err)
	}

	// Create storage latency histogram
	runStorageLatency, err = meter.Float64Histogram(
		"run_storage_latency_seconds",
		metric.WithDescription("time from run completion to successful storage"),
		metric.WithExplicitBucketBoundaries(0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300, 600, 1800),
	)
	if err != nil {
		return fmt.Errorf("failed to create run_storage_latency_seconds histogram: %w", err)
	}

	logger.Debug("initialized shared metrics instruments")
	return nil
}

// EnsureMetricsInitialized ensures all shared metrics are initialized exactly once.
// Subsequent calls are no-ops. This is safe to call multiple times from any goroutine.
func EnsureMetricsInitialized(logger *zap.SugaredLogger) {
	once.Do(func() {
		if err := initializeMetrics(logger); err != nil {
			logger.Errorf("Failed to initialize metrics: %v", err)
		}
	})
}

// CountRunNotStored records a run that was not stored due to deletion or timeout
func CountRunNotStored(ctx context.Context, namespace, kind string) error {
	if runsNotStoredCount == nil {
		return fmt.Errorf("runsNotStoredCount metric is not initialized")
	}

	runsNotStoredCount.Add(ctx, 1,
		metric.WithAttributes(
			kindKey.String(kind),
			namespaceKey.String(namespace),
		),
	)
	return nil
}

// RecordStorageLatency records the storage latency metric for both PipelineRuns and TaskRuns
func (r *Recorder) RecordStorageLatency(ctx context.Context, object interface{}) error {
	if runStorageLatency == nil {
		return fmt.Errorf("runStorageLatency metric is not initialized")
	}

	var (
		completionTime *time.Time
		namespace      string
		kind           string
	)

	// Extract completion time and metadata using type switch
	switch o := object.(type) {
	case *pipelinev1.PipelineRun:
		if o.Status.CompletionTime == nil {
			return nil
		}
		completionTime = &o.Status.CompletionTime.Time
		namespace = o.Namespace
		kind = "pipelinerun"

	case *pipelinev1.TaskRun:
		if o.Status.CompletionTime == nil {
			return nil
		}
		completionTime = &o.Status.CompletionTime.Time
		namespace = o.Namespace
		kind = "taskrun"

	default:
		return fmt.Errorf("unsupported object type: %T", object)
	}

	// Calculate latency from completion to now
	now := r.clock.Now()
	latency := now.Sub(*completionTime)

	// Record the metric
	runStorageLatency.Record(ctx, latency.Seconds(),
		metric.WithAttributes(
			kindKey.String(kind),
			namespaceKey.String(namespace),
		),
	)

	return nil
}
