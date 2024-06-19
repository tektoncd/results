package taskrunmetrics

import (
	"context"
	"errors"
	"time"

	"github.com/jonboulle/clockwork"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
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
	trDeleteCount        = stats.Int64("taskrun_delete_count", "total number of deleted taskruns", stats.UnitDimensionless)
	trDeleteCountView    *view.View
	trDeleteDuration     = stats.Float64("taskrun_delete_duration_seconds", "the pipelinerun deletion time in seconds", stats.UnitSeconds)
	trDeleteDurationView *view.View
	pipelineTag          = tag.MustNewKey("pipeline")
	taskTag              = tag.MustNewKey("task")
	namespaceTag         = tag.MustNewKey("namespace")
	statusTag            = tag.MustNewKey("status")
)

// Recorder is used to actually record TaskRun metrics
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
	switch cfg.TaskrunLevel {
	case config.TaskrunLevelAtTask:
		tags = []tag.Key{taskTag}
	case config.TaskrunLevelAtNS:
		tags = []tag.Key{}
	default:
		return errors.New("invalid config for TaskrunLevel: " + cfg.TaskrunLevel)
	}

	switch cfg.PipelinerunLevel {
	case config.PipelinerunLevelAtPipeline:
		tags = append(tags, pipelineTag)
	case config.PipelinerunLevelAtNS:
	default:
		return errors.New("invalid config for PipelinerunLevel: " + cfg.PipelinerunLevel)
	}

	trDeleteCountView = &view.View{
		Description: trDeleteCount.Description(),
		TagKeys:     []tag.Key{statusTag, namespaceTag},
		Measure:     trDeleteCount,
		Aggregation: view.Count(),
	}

	var distribution *view.Aggregation
	switch cfg.DurationPipelinerunType {
	case config.DurationTaskrunTypeLastValue:
		distribution = view.LastValue()
	case config.DurationTaskrunTypeHistogram:
		distribution = view.Distribution(10, 30, 60, 300, 900, 1800, 3600, 5400, 10800, 21600, 43200, 86400)
	}

	trDeleteDurationView = &view.View{
		Description: trDeleteDuration.Description(),
		TagKeys:     append([]tag.Key{statusTag, namespaceTag}, tags...),
		Measure:     trDeleteDuration,
		Aggregation: distribution,
	}
	logger.Debug("registering taskrun metrics view")
	return view.Register(trDeleteCountView, trDeleteDurationView)
}

func viewUnregister(logger *zap.SugaredLogger) {
	logger.Debug("unregistering taskrun metrics view")
	if trDeleteCountView != nil {
		view.Unregister(trDeleteCountView)
	}
	if trDeleteDurationView != nil {
		view.Unregister(trDeleteDurationView)
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

// DurationAndCountDeleted counts deleted number and record duration for TaskRuns
func (r *Recorder) DurationAndCountDeleted(ctx context.Context, cfg *config.Metrics, tr *pipelinev1.TaskRun) error {
	taskName := "anonymous"
	pipelineName := "anonymous"
	now := r.clock.Now()

	if tr.Spec.TaskRef != nil && tr.Spec.TaskRef.Name != "" {
		taskName = tr.Spec.TaskRef.Name
	}

	status := "success"
	deleteDuration := time.Duration(0)
	if cond := tr.Status.GetCondition(apis.ConditionSucceeded); cond.Status == corev1.ConditionFalse {
		status = "failed"

		// Use failedTime to compute delete duration in case of completion time being nil
		failedTime := cond.LastTransitionTime.Inner.Time
		if !failedTime.After(now) {
			deleteDuration = now.Sub(failedTime)
		}
		if cond.Reason == pipelinev1.TaskRunSpecStatusCancelled {
			status = "cancelled"
		}
	}

	tags := []tag.Mutator{tag.Insert(namespaceTag, tr.Namespace), tag.Insert(statusTag, status)}

	if ok, pipeline, _ := isPartOfPipeline(tr); ok {
		pipelineName = pipeline
	}

	tags = append(tags, r.insertPipelineTag(cfg, pipelineName)...)
	tags = append(tags, r.insertTaskTag(cfg, taskName)...)

	ctx, err := tag.New(ctx, tags...)
	if err != nil {
		return err
	}

	if tr.Status.CompletionTime != nil && !tr.Status.CompletionTime.Time.After(now) {
		deleteDuration = now.Sub(tr.Status.CompletionTime.Time)
	}
	metrics.Record(ctx, trDeleteCount.M(1))
	metrics.Record(ctx, trDeleteDuration.M(float64(deleteDuration/time.Second)))
	return nil
}

func (r *Recorder) insertPipelineTag(cfg *config.Metrics, pipeline string) []tag.Mutator {
	var tags []tag.Mutator
	switch cfg.PipelinerunLevel {
	case config.PipelinerunLevelAtPipeline:
		tags = []tag.Mutator{tag.Insert(pipelineTag, pipeline)}
	case config.PipelinerunLevelAtNS:
	}
	return tags
}

func (r *Recorder) insertTaskTag(cfg *config.Metrics, task string) []tag.Mutator {
	var tags []tag.Mutator
	switch cfg.TaskrunLevel {
	case config.TaskrunLevelAtTask:
		tags = []tag.Mutator{tag.Insert(taskTag, task)}
	case config.TaskrunLevelAtNS:
	}
	return tags
}

// IsPartOfPipeline return true if TaskRun is a part of a Pipeline.
// It also returns the name of Pipeline and PipelineRun
func isPartOfPipeline(tr *pipelinev1.TaskRun) (bool, string, string) {
	pipelineLabel, hasPipelineLabel := tr.Labels[pipeline.PipelineLabelKey]
	pipelineRunLabel, hasPipelineRunLabel := tr.Labels[pipeline.PipelineRunLabelKey]

	if hasPipelineLabel && hasPipelineRunLabel {
		return true, pipelineLabel, pipelineRunLabel
	}

	return false, "", ""
}
