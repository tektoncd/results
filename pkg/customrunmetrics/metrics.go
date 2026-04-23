// Package customrunmetrics provides metrics collection for CustomRun resources.
package customrunmetrics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	pipelinev1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
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
	crDeleteCount    metric.Int64Counter
	crDeleteDuration metric.Float64Histogram

	// Attribute keys
	pipelineKey  = attribute.Key("pipeline")
	customRunKey = attribute.Key("customrun")
	namespaceKey = attribute.Key("namespace")
	statusKey    = attribute.Key("status")
)

// Recorder is used to actually record CustomRun metrics
type Recorder struct {
	clock clockwork.Clock
}

// NewRecorder creates a new metrics recorder instance
// to log the CustomRun related metrics
func NewRecorder(_ context.Context) (*Recorder, error) {
	logger := zap.S()
	if ctxLogger, err := zap.NewProduction(); err == nil {
		logger = ctxLogger.Sugar()
	}

	var errRegistering error
	once.Do(func() {
		errRegistering = initializeMetrics(logger)
		if errRegistering != nil {
			logger.Errorf("Failed to initialize customrun metrics: %v", errRegistering)
		}
	})

	return &Recorder{clock: clockwork.NewRealClock()}, errRegistering
}

func initializeMetrics(logger *zap.SugaredLogger) error {
	meter := otel.Meter("tekton.dev/results/customrun")

	var err error

	// Create delete count counter
	// Note: OpenTelemetry Prometheus exporter will append "_total" suffix to counter metrics
	// So this will be exported as "watcher_customrun_delete_count_total"
	crDeleteCount, err = meter.Int64Counter(
		"watcher_customrun_delete_count",
		metric.WithDescription("total number of deleted customruns"),
	)
	if err != nil {
		return fmt.Errorf("failed to create watcher_customrun_delete_count counter: %w", err)
	}

	// Create delete duration histogram
	// Use histogram for duration tracking (histogram is more appropriate than last value for duration metrics in OTel)
	crDeleteDuration, err = meter.Float64Histogram(
		"watcher_customrun_delete_duration_seconds",
		metric.WithDescription("the customrun deletion time in seconds"),
		metric.WithExplicitBucketBoundaries(10, 30, 60, 300, 900, 1800, 3600, 5400, 10800, 21600, 43200, 86400),
	)
	if err != nil {
		return fmt.Errorf("failed to create watcher_customrun_delete_duration_seconds histogram: %w", err)
	}

	logger.Debug("initialized customrun metrics instruments")
	return nil
}

// DurationAndCountDeleted counts deleted number and record duration for CustomRuns
func (r *Recorder) DurationAndCountDeleted(ctx context.Context, cfg *config.Metrics, cr *pipelinev1beta1.CustomRun) error {
	if crDeleteCount == nil || crDeleteDuration == nil {
		return fmt.Errorf("customrun metrics are not initialized")
	}

	customRunName := "anonymous"
	pipelineName := "anonymous"
	now := r.clock.Now()

	if cr.Spec.CustomRef != nil && cr.Spec.CustomRef.Name != "" {
		customRunName = cr.Spec.CustomRef.Name
	}

	status := "success"
	deleteDuration := time.Duration(0)
	if cond := cr.Status.GetCondition(apis.ConditionSucceeded); cond.Status == corev1.ConditionFalse {
		status = "failed"

		// Use failedTime to compute delete duration in case of completion time being nil
		failedTime := cond.LastTransitionTime.Inner.Time
		if !failedTime.After(now) {
			deleteDuration = now.Sub(failedTime)
		}
		if cond.Reason == pipelinev1beta1.CustomRunReasonCancelled.String() {
			status = "cancelled"
		}
	}

	if ok, pipeline, _ := isPartOfPipeline(cr); ok {
		pipelineName = pipeline
	}

	if cr.Status.CompletionTime != nil && !cr.Status.CompletionTime.After(now) {
		deleteDuration = now.Sub(cr.Status.CompletionTime.Time)
	}

	// Build attributes
	attrs := []attribute.KeyValue{
		namespaceKey.String(cr.Namespace),
		statusKey.String(status),
	}
	attrs = append(attrs, r.insertPipelineAttr(cfg, pipelineName)...)
	attrs = append(attrs, r.insertCustomRunAttr(cfg, customRunName)...)

	// Record metrics
	crDeleteCount.Add(ctx, 1, metric.WithAttributes(attrs...))
	crDeleteDuration.Record(ctx, deleteDuration.Seconds(), metric.WithAttributes(attrs...))

	return nil
}

func (r *Recorder) insertPipelineAttr(cfg *config.Metrics, pipeline string) []attribute.KeyValue {
	var attrs []attribute.KeyValue
	switch cfg.PipelinerunLevel {
	case config.PipelinerunLevelAtPipeline:
		attrs = []attribute.KeyValue{pipelineKey.String(pipeline)}
	case config.PipelinerunLevelAtNS:
	}
	return attrs
}

func (r *Recorder) insertCustomRunAttr(cfg *config.Metrics, customRun string) []attribute.KeyValue {
	var attrs []attribute.KeyValue
	switch cfg.TaskrunLevel {
	case config.TaskrunLevelAtTask:
		attrs = []attribute.KeyValue{customRunKey.String(customRun)}
	case config.TaskrunLevelAtNS:
	}
	return attrs
}

// isPartOfPipeline return true if CustomRun is a part of a Pipeline.
// It also returns the name of Pipeline and PipelineRun
func isPartOfPipeline(cr *pipelinev1beta1.CustomRun) (bool, string, string) {
	pipelineLabel, hasPipelineLabel := cr.Labels[pipeline.PipelineLabelKey]
	pipelineRunLabel, hasPipelineRunLabel := cr.Labels[pipeline.PipelineRunLabelKey]

	if hasPipelineLabel && hasPipelineRunLabel {
		return true, pipelineLabel, pipelineRunLabel
	}

	return false, "", ""
}
