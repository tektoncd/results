// Package pipelinerunmetrics provides metrics collection for PipelineRun resources.
package pipelinerunmetrics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tektoncd/results/pkg/apis/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"knative.dev/pkg/apis"
)

var (
	once sync.Once

	// OpenTelemetry instruments
	prDeleteCount    metric.Int64Counter
	prDeleteDuration metric.Float64Histogram

	// Attribute keys
	pipelineKey  = attribute.Key("pipeline")
	namespaceKey = attribute.Key("namespace")
	statusKey    = attribute.Key("status")
)

// Recorder is used to actually record PipelineRun metrics
type Recorder struct {
	clock clockwork.Clock
}

// NewRecorder creates a new metrics recorder instance
// to log the PipelineRun related metrics
func NewRecorder(_ context.Context) (*Recorder, error) {
	logger := zap.S()
	if ctxLogger, err := zap.NewProduction(); err == nil {
		logger = ctxLogger.Sugar()
	}

	var errRegistering error
	once.Do(func() {
		errRegistering = initializeMetrics(logger)
		if errRegistering != nil {
			logger.Errorf("Failed to initialize pipelinerun metrics: %v", errRegistering)
		}
	})

	return &Recorder{clock: clockwork.NewRealClock()}, errRegistering
}

func initializeMetrics(logger *zap.SugaredLogger) error {
	meter := otel.Meter("tekton.dev/results/pipelinerun")

	var err error

	// Create delete count counter
	// Note: OpenTelemetry Prometheus exporter will append "_total" suffix to counter metrics
	// So this will be exported as "watcher_pipelinerun_delete_count_total"
	prDeleteCount, err = meter.Int64Counter(
		"watcher_pipelinerun_delete_count",
		metric.WithDescription("total number of deleted pipelineruns"),
	)
	if err != nil {
		return fmt.Errorf("failed to create watcher_pipelinerun_delete_count counter: %w", err)
	}

	// Create delete duration histogram
	// Use histogram for duration tracking (histogram is more appropriate than last value for duration metrics in OTel)
	prDeleteDuration, err = meter.Float64Histogram(
		"watcher_pipelinerun_delete_duration_seconds",
		metric.WithDescription("the pipelinerun deletion time in seconds"),
		metric.WithExplicitBucketBoundaries(10, 30, 60, 300, 900, 1800, 3600, 5400, 10800, 21600, 43200, 86400),
	)
	if err != nil {
		return fmt.Errorf("failed to create watcher_pipelinerun_delete_duration_seconds histogram: %w", err)
	}

	logger.Debug("initialized pipelinerun metrics instruments")
	return nil
}

// DurationAndCountDeleted counts for deleted number and records duration PipelineRuns
func (r *Recorder) DurationAndCountDeleted(ctx context.Context, cfg *config.Metrics, pr *pipelinev1.PipelineRun) error {
	if prDeleteCount == nil || prDeleteDuration == nil {
		return fmt.Errorf("pipelinerun metrics are not initialized")
	}

	pipelineName := "anonymous"
	now := r.clock.Now()

	if pr.Spec.PipelineRef != nil && pr.Spec.PipelineRef.Name != "" {
		pipelineName = pr.Spec.PipelineRef.Name
	}

	// Metrics status tag meaning should be consistent with pipeline repo definition
	// TODO(xinnjie) metrics status query function should be defined in pipeline repo, and use that function here
	status := "success"
	deleteDuration := time.Duration(0)

	if cond := pr.Status.GetCondition(apis.ConditionSucceeded); cond.Status == corev1.ConditionFalse {
		status = "failed"
		// Use failedTime to compute delete duration in case of completion time being nil
		failedTime := cond.LastTransitionTime.Inner.Time
		if !failedTime.After(now) {
			deleteDuration = now.Sub(failedTime)
		}
		if cond.Reason == "Cancelled" {
			status = "cancelled"
		}
	}

	if pr.Status.CompletionTime != nil && !pr.Status.CompletionTime.After(now) {
		deleteDuration = now.Sub(pr.Status.CompletionTime.Time)
	}

	// Build attributes
	attrs := []attribute.KeyValue{
		namespaceKey.String(pr.Namespace),
		statusKey.String(status),
	}
	if cfg.PipelinerunLevel == config.PipelinerunLevelAtPipeline {
		attrs = append(attrs, pipelineKey.String(pipelineName))
	}

	// Record metrics
	prDeleteCount.Add(ctx, 1, metric.WithAttributes(attrs...))
	prDeleteDuration.Record(ctx, deleteDuration.Seconds(), metric.WithAttributes(attrs...))

	return nil
}
