package metrics

import (
	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	"time"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

// ExtractPipelineRunMetadata extracts completion time and metadata from a PipelineRun for storage latency metrics
func ExtractPipelineRunMetadata(pr *pipelinev1.PipelineRun) (*time.Time, string, string) {
	if pr.Status.CompletionTime == nil {
		// No completion time, the PipelineRun hasn't completed yet
		return nil, "", ""
	}

	completionTime := &pr.Status.CompletionTime.Time
	namespace := pr.Namespace

	var pipelineName string
	if pr.Spec.PipelineRef != nil && pr.Spec.PipelineRef.Name != "" {
		pipelineName = pr.Spec.PipelineRef.Name
	} else {
		pipelineName = "anonymous"
	}

	return completionTime, namespace, pipelineName
}

// ExtractTaskRunMetadata extracts completion time and metadata from a TaskRun for storage latency metrics
func ExtractTaskRunMetadata(tr *pipelinev1.TaskRun) (*time.Time, string, string, string) {
	if tr.Status.CompletionTime == nil {
		// No completion time, the TaskRun hasn't completed yet
		return nil, "", "", ""
	}

	completionTime := &tr.Status.CompletionTime.Time
	namespace := tr.Namespace

	// Get the individual task name within the pipeline (e.g., "first-add", "second-add")
	var taskName string
	if pipelineTaskName, ok := tr.Labels["tekton.dev/pipelineTask"]; ok {
		taskName = pipelineTaskName
	} else if tr.Spec.TaskRef != nil && tr.Spec.TaskRef.Name != "" {
		// Fall back to task template name if not part of a pipeline
		taskName = tr.Spec.TaskRef.Name
	} else {
		taskName = "anonymous"
	}

	// For TaskRuns, try to get pipeline name if it's part of a pipeline
	var pipelineName string
	if ok, pipeline, _ := isPartOfPipeline(tr); ok {
		pipelineName = pipeline
	} else {
		pipelineName = "anonymous"
	}

	return completionTime, namespace, pipelineName, taskName
}

// isPartOfPipeline return true if TaskRun is a part of a Pipeline.
// It also returns the name of Pipeline and PipelineRun
func isPartOfPipeline(tr *pipelinev1.TaskRun) (bool, string, string) {
	pipelineLabel, hasPipelineLabel := tr.Labels[pipeline.PipelineLabelKey]
	pipelineRunLabel, hasPipelineRunLabel := tr.Labels[pipeline.PipelineRunLabelKey]

	if hasPipelineLabel && hasPipelineRunLabel {
		return true, pipelineLabel, pipelineRunLabel
	}

	return false, "", ""
}
