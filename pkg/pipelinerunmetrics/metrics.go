package pipelinerunmetrics

import (
	"context"
	"errors"
	"time"

	"github.com/jonboulle/clockwork"

	pipelinev1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/results/pkg/apis/config"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/metrics"
)

var (
	prDeleteCount        = stats.Int64("pipelinerun_delete_count", "total number of deleted pipelineruns", stats.UnitDimensionless)
	prDeleteCountView    *view.View
	prDeleteDuration     = stats.Float64("pipelinerun_delete_duration_seconds", "the pipelinerun deletion time in seconds", stats.UnitSeconds)
	prDeleteDurationView *view.View
	pipelineTag          = tag.MustNewKey("pipeline")
	namespaceTag         = tag.MustNewKey("namespace")
	statusTag            = tag.MustNewKey("status")
)

// Recorder is used to actually record PipelineRun metrics
type Recorder struct {
	clock clockwork.Clock
}

// NewRecorder creates a new metrics recorder instance
// to log the PipelineRun related metrics
func NewRecorder() *Recorder {
	return &Recorder{clock: clockwork.NewRealClock()}
}

func viewRegister(logger *zap.SugaredLogger, cfg *config.Metrics) error {
	var tags []tag.Key
	switch cfg.PipelinerunLevel {
	case config.PipelinerunLevelAtPipeline:
		tags = []tag.Key{pipelineTag}
	case config.PipelinerunLevelAtNS:
		tags = []tag.Key{}
	default:
		return errors.New("invalid config for PipelinerunLevel: " + cfg.PipelinerunLevel)
	}
	prDeleteCountView = &view.View{
		Description: prDeleteCount.Description(),
		TagKeys:     []tag.Key{statusTag, namespaceTag},
		Measure:     prDeleteCount,
		Aggregation: view.Count(),
	}

	var distribution *view.Aggregation
	switch cfg.DurationPipelinerunType {
	case config.DurationPipelinerunTypeLastValue:
		distribution = view.LastValue()
	case config.DurationPipelinerunTypeHistogram:
		distribution = view.Distribution(10, 30, 60, 300, 900, 1800, 3600, 5400, 10800, 21600, 43200, 86400)
	}

	prDeleteDurationView = &view.View{
		Description: prDeleteCount.Description(),
		TagKeys:     append([]tag.Key{statusTag, namespaceTag}, tags...),
		Measure:     prDeleteDuration,
		Aggregation: distribution,
	}
	logger.Debug("registering pipelinerun metrics view")
	return view.Register(prDeleteCountView, prDeleteDurationView)
}

func viewUnregister(logger *zap.SugaredLogger) {
	logger.Debug("unregistering pipelinerun metrics view")
	if prDeleteCountView != nil {
		view.Unregister(prDeleteCountView)
	}

	if prDeleteDurationView != nil {
		view.Unregister(prDeleteDurationView)
	}
}

// MetricsOnStore returns a function that checks if metrics are configured for a config.Store, and registers it if so
func MetricsOnStore(logger *zap.SugaredLogger) func(name string,
	value any) {
	return func(name string, value any) {
		if name != config.GetMetricsConfigName() {
			return
		}
		cfg, ok := value.(*config.Metrics)
		if !ok {
			logger.Error("Failed to do type insertion for extracting metrics config")
			return
		}
		viewUnregister(logger)
		err := viewRegister(logger, cfg)
		if err != nil {
			logger.Errorf("Failed to register View %v ", err)
			return
		}
	}
}

// DurationAndCountDeleted counts for deleted number and records duration PipelineRuns
func (r *Recorder) DurationAndCountDeleted(ctx context.Context, cfg *config.Metrics, pr *pipelinev1beta1.PipelineRun) error {
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

	var tags []tag.Mutator
	if cfg.PipelinerunLevel == config.PipelinerunLevelAtPipeline {
		tags = []tag.Mutator{tag.Insert(pipelineTag, pipelineName)}
	}
	ctx, err := tag.New(ctx, append([]tag.Mutator{tag.Insert(namespaceTag, pr.Namespace), tag.Insert(statusTag, status)}, tags...)...)
	if err != nil {
		return err
	}

	if pr.Status.CompletionTime != nil && !pr.Status.CompletionTime.After(now) {
		deleteDuration = now.Sub(pr.Status.CompletionTime.Time)
	}

	metrics.Record(ctx, prDeleteCount.M(1))
	metrics.Record(ctx, prDeleteDuration.M(float64(deleteDuration/time.Second)))
	return nil
}
