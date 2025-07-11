package metrics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tektoncd/results/pkg/apis/config"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"go.uber.org/zap"
	"knative.dev/pkg/metrics"
)

// RunType represents the type of Tekton run (PipelineRun or TaskRun)
type RunType string

const (
	// PipelineRunType represents a PipelineRun resource
	PipelineRunType RunType = "pipelinerun"
	// TaskRunType represents a TaskRun resource
	TaskRunType RunType = "taskrun"
)

var (
	registerMutex sync.Mutex

	registeredAt       *time.Time
	runsNotStoredCount = stats.Int64("runs_not_stored_count", "total number of runs which were deleted without being stored", stats.UnitDimensionless)
	runsNotStoredView  *view.View

	// Storage latency metric (shared)
	runStorageLatency     = stats.Float64("run_storage_latency_seconds", "time from run completion to successful storage", stats.UnitSeconds)
	runStorageLatencyView *view.View

	// Common tags
	pipelineTag  = tag.MustNewKey("pipeline")
	taskTag      = tag.MustNewKey("task")
	namespaceTag = tag.MustNewKey("namespace")
	kindTag      = tag.MustNewKey("kind")
)

// Recorder is used to record metrics for both PipelineRuns and TaskRuns
type Recorder struct {
	clock clockwork.Clock
}

// NewRecorder creates a new metrics recorder instance
func NewRecorder() *Recorder {
	return &Recorder{clock: clockwork.NewRealClock()}
}

func registerViews(logger *zap.SugaredLogger) error {
	runsNotStoredView = &view.View{
		Description: runsNotStoredCount.Description(),
		TagKeys:     []tag.Key{kindTag, namespaceTag},
		Measure:     runsNotStoredCount,
		Aggregation: view.Count(),
	}

	// Storage latency view
	runStorageLatencyView = &view.View{
		Description: runStorageLatency.Description(),
		TagKeys:     []tag.Key{kindTag, namespaceTag, pipelineTag, taskTag},
		Measure:     runStorageLatency,
		Aggregation: view.Distribution(0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300, 600, 1800),
	}

	logger.Debug("registering shared metrics views")
	return view.Register(runsNotStoredView, runStorageLatencyView)
}

func unregisterViews(logger *zap.SugaredLogger) {
	logger.Debug("unregistering shared metrics views")
	var viewsToUnregister []*view.View
	if runsNotStoredView != nil {
		viewsToUnregister = append(viewsToUnregister, runsNotStoredView)
	}
	if runStorageLatencyView != nil {
		viewsToUnregister = append(viewsToUnregister, runStorageLatencyView)
	}
	view.Unregister(viewsToUnregister...)
	registeredAt = nil
}

// IdempotentRegisterViews Ensures all shared views are registered.
// Does not unregister views if they have already been registered.
func IdempotentRegisterViews(logger *zap.SugaredLogger) {
	registerMutex.Lock()
	defer registerMutex.Unlock()
	if registeredAt != nil {
		return
	}
	unregisterViews(logger)
	if err := registerViews(logger); err != nil {
		logger.Errorf("Failed to register View %v ", err)
	} else {
		now := time.Now()
		registeredAt = &now
	}
}

// CountRunNotStored records a run that was not stored due to deletion or timeout
func CountRunNotStored(ctx context.Context, namespace, kind string) error {
	ctx, err := tag.New(
		ctx,
		tag.Insert(kindTag, kind),
		tag.Insert(namespaceTag, namespace),
	)
	if err != nil {
		return fmt.Errorf("unable to create tags for %s metric: %w", runsNotStoredCount.Name(), err)
	}

	metrics.Record(ctx, runsNotStoredCount.M(1))
	return nil
}

// OnStore returns a function that checks if metrics are configured for a config.Store, and registers it if so
func OnStore(logger *zap.SugaredLogger) func(name string, value any) {
	return func(name string, value any) {
		if name != config.GetMetricsConfigName() {
			return
		}
		_, ok := value.(*config.Metrics)
		if !ok {
			logger.Error("Failed to do type insertion for extracting metrics config")
			return
		}
		// For shared metrics, we use idempotent registration
		IdempotentRegisterViews(logger)
	}
}

// RecordStorageLatency records the storage latency metric for both PipelineRuns and TaskRuns
// runType indicates whether this is a PipelineRun or TaskRun
func (r *Recorder) RecordStorageLatency(ctx context.Context, runType RunType, object interface{}) error {
	var (
		completionTime *time.Time
		namespace      string
		pipelineName   string
		taskName       string
	)

	// Extract completion time and metadata based on run type
	switch runType {
	case PipelineRunType:
		pr, ok := object.(*pipelinev1.PipelineRun)
		if !ok {
			return fmt.Errorf("object is not a PipelineRun")
		}

		completionTime, namespace, pipelineName = ExtractPipelineRunMetadata(pr)
		if completionTime == nil {
			// No completion time, the PipelineRun hasn't completed yet
			return nil
		}

	case TaskRunType:
		tr, ok := object.(*pipelinev1.TaskRun)
		if !ok {
			return fmt.Errorf("object is not a TaskRun")
		}

		completionTime, namespace, pipelineName, taskName = ExtractTaskRunMetadata(tr)
		if completionTime == nil {
			// No completion time, the TaskRun hasn't completed yet
			return nil
		}

	default:
		return fmt.Errorf("unsupported run type: %s", runType)
	}

	// Calculate latency from completion to now
	now := r.clock.Now()
	latency := now.Sub(*completionTime)

	// Create tags
	tags := []tag.Mutator{
		tag.Insert(kindTag, string(runType)),
		tag.Insert(namespaceTag, namespace),
		tag.Insert(pipelineTag, pipelineName),
		tag.Insert(taskTag, taskName),
	}

	ctx, err := tag.New(ctx, tags...)
	if err != nil {
		return fmt.Errorf("error creating tagged context: %w", err)
	}

	// Record the metric
	metrics.Record(ctx, runStorageLatency.M(float64(latency/time.Second)))

	return nil
}
