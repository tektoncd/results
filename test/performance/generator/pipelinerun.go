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

package generator

import (
	"fmt"
	"time"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

const (
	pipelineRunDuration = 12 * time.Minute
	taskRunDuration     = 8 * time.Minute
)

// buildPipelineRun deep-copies the template PipelineRun and overwrites the
// identity, label, timestamp and outcome fields for this instance.
func (g *Generator) buildPipelineRun(tmpl *Template, inst *Instance, start time.Time) *tektonv1.PipelineRun {
	pr := tmpl.pipelineRun.DeepCopy()
	pr.TypeMeta = metav1.TypeMeta{APIVersion: "tekton.dev/v1", Kind: "PipelineRun"}
	pr.Name = fmt.Sprintf("pr-%s", inst.UID)
	pr.Namespace = inst.Namespace
	pr.UID = types.UID(inst.UID)
	pr.CreationTimestamp = metav1.NewTime(start)
	applyLabels(&pr.ObjectMeta, inst.Labels)

	end := start.Add(pipelineRunDuration)
	pr.Status.StartTime = &metav1.Time{Time: start}
	pr.Status.CompletionTime = &metav1.Time{Time: end}
	pr.Status.Conditions = terminalCondition(inst.Outcome, end)
	return pr
}

// buildTaskRuns synthesizes the child TaskRun records for an instance and
// rewrites the PipelineRun's childReferences to match them.
func (g *Generator) buildTaskRuns(tmpl *Template, inst *Instance, index, count int, start time.Time) []*tektonv1.TaskRun {
	skel := tmpl.taskRun
	if skel == nil {
		skel = defaultTaskRunSkeleton()
	}

	trs := make([]*tektonv1.TaskRun, 0, count)
	refs := make([]tektonv1.ChildStatusReference, 0, count)
	for j := 0; j < count; j++ {
		childUID := g.cfg.UIDs.childUID(index, j)
		tr := skel.DeepCopy()
		tr.TypeMeta = metav1.TypeMeta{APIVersion: "tekton.dev/v1", Kind: "TaskRun"}
		name := fmt.Sprintf("tr-%s", childUID)
		tr.Name = name
		tr.Namespace = inst.Namespace
		tr.UID = types.UID(childUID)
		tr.CreationTimestamp = metav1.NewTime(start)
		applyLabels(&tr.ObjectMeta, inst.Labels)
		tr.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: "tekton.dev/v1",
			Kind:       "PipelineRun",
			Name:       inst.PipelineRun.Name,
			UID:        types.UID(inst.UID),
		}}

		trStart := start.Add(time.Duration(j) * time.Minute)
		trEnd := trStart.Add(taskRunDuration)
		tr.Status.StartTime = &metav1.Time{Time: trStart}
		tr.Status.CompletionTime = &metav1.Time{Time: trEnd}
		// Only the last child inherits a non-successful outcome, mirroring how a
		// failed/cancelled PipelineRun typically has one culprit TaskRun.
		childOutcome := OutcomeSucceeded
		if j == count-1 {
			childOutcome = inst.Outcome
		}
		tr.Status.Conditions = terminalCondition(childOutcome, trEnd)

		trs = append(trs, tr)
		refs = append(refs, tektonv1.ChildStatusReference{
			TypeMeta:         runtime.TypeMeta{APIVersion: "tekton.dev/v1", Kind: "TaskRun"},
			Name:             name,
			PipelineTaskName: fmt.Sprintf("task-%d", j),
		})
	}
	inst.PipelineRun.Status.ChildReferences = refs
	return trs
}

// applyLabels overlays the drawn label subset onto the object's existing labels.
func applyLabels(meta *metav1.ObjectMeta, labels map[string]string) {
	if meta.Labels == nil {
		meta.Labels = map[string]string{}
	}
	for k, v := range labels {
		meta.Labels[k] = v
	}
}

// terminalCondition returns the Succeeded condition for a terminal outcome.
func terminalCondition(o Outcome, at time.Time) duckv1.Conditions {
	c := apis.Condition{
		Type:               apis.ConditionSucceeded,
		LastTransitionTime: apis.VolatileTime{Inner: metav1.NewTime(at)},
	}
	switch o {
	case OutcomeSucceeded:
		c.Status = corev1.ConditionTrue
		c.Reason = "Succeeded"
		c.Message = "Tasks Completed successfully"
	case OutcomeFailed:
		c.Status = corev1.ConditionFalse
		c.Reason = "Failed"
		c.Message = "Tasks Completed with failures"
	case OutcomeCancelled:
		c.Status = corev1.ConditionFalse
		c.Reason = "Cancelled"
		c.Message = "PipelineRun was cancelled"
	}
	return duckv1.Conditions{c}
}

// defaultTaskRunSkeleton returns a minimal TaskRun used when a template omits
// taskrun.yaml.
func defaultTaskRunSkeleton() *tektonv1.TaskRun {
	return &tektonv1.TaskRun{
		Spec: tektonv1.TaskRunSpec{
			TaskRef: &tektonv1.TaskRef{Name: "generic-task"},
		},
	}
}
