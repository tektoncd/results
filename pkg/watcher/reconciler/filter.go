// Copyright 2026 The Tekton Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package reconciler

import (
	"slices"
	"strings"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	v1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
)

// Finalizer names added by the watcher to track resources it processes.
const (
	PipelineRunFinalizer = "results.tekton.dev/pipelinerun"
	TaskRunFinalizer     = "results.tekton.dev/taskrun"
)

// IsManagedByAllowed reports whether a spec.managedBy value should be
// processed. Nil, empty, and whitespace-only values are always allowed.
// The incoming value is trimmed before lookup so that " custom-controller "
// matches a configured "custom-controller".
func IsManagedByAllowed(managedBy *string, allowed sets.Set[string]) bool {
	if managedBy == nil {
		return true
	}
	trimmed := strings.TrimSpace(*managedBy)
	if trimmed == "" || trimmed == pipeline.ManagedBy {
		return true
	}
	return allowed.Has(trimmed)
}

// ParseManagedByValues parses a comma-separated list of additional
// spec.managedBy values into a set. pipeline.ManagedBy is always included.
func ParseManagedByValues(raw string) sets.Set[string] {
	s := sets.New[string](pipeline.ManagedBy)
	if raw == "" {
		return s
	}
	for _, v := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			s.Insert(trimmed)
		}
	}
	return s
}

// unwrapTombstone extracts the wrapped object from a
// cache.DeletedFinalStateUnknown tombstone. If obj is not a tombstone
// it is returned as-is.
func unwrapTombstone(obj interface{}) interface{} {
	if d, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		return d.Obj
	}
	return obj
}

// PipelineRunFilterFunc returns a filter function for PipelineRuns
// that checks whether the run's spec.managedBy value is allowed.
// Tombstone (DeletedFinalStateUnknown) objects are unwrapped before
// the type assertion so that delete events are never silently dropped.
// Objects being deleted (DeletionTimestamp set) that carry a Results
// finalizer are always passed through so the finalizer can be removed
// even if the allowlist has been shrunk since the finalizer was added.
func PipelineRunFilterFunc(allowedManagedBy sets.Set[string]) func(obj interface{}) bool {
	return func(obj interface{}) bool {
		pr, ok := unwrapTombstone(obj).(*v1.PipelineRun)
		if !ok || pr == nil {
			return false
		}
		if pr.DeletionTimestamp != nil && slices.Contains(pr.Finalizers, PipelineRunFinalizer) {
			return true
		}
		return IsManagedByAllowed(pr.Spec.ManagedBy, allowedManagedBy)
	}
}

// TaskRunFilterFunc returns a filter function for TaskRuns
// that checks whether the run's spec.managedBy value is allowed.
// Tombstone (DeletedFinalStateUnknown) objects are unwrapped before
// the type assertion so that delete events are never silently dropped.
// Objects being deleted (DeletionTimestamp set) that carry a Results
// finalizer are always passed through so the finalizer can be removed
// even if the allowlist has been shrunk since the finalizer was added.
func TaskRunFilterFunc(allowedManagedBy sets.Set[string]) func(obj interface{}) bool {
	return func(obj interface{}) bool {
		tr, ok := unwrapTombstone(obj).(*v1.TaskRun)
		if !ok || tr == nil {
			return false
		}
		if tr.DeletionTimestamp != nil && slices.Contains(tr.Finalizers, TaskRunFinalizer) {
			return true
		}
		return IsManagedByAllowed(tr.Spec.ManagedBy, allowedManagedBy)
	}
}
