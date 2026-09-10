// Copyright 2021 The Tekton Authors
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

// Package reconciler provides the core reconciliation logic for the watcher.
package reconciler

import (
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
)

// Config defines shared reconciler configuration options.
type Config struct {
	// Configures whether Tekton CRD objects should be updated with Result
	// annotations during reconcile. Useful to enable for dry run modes.
	DisableAnnotationUpdate bool

	// CompletedResourceGracePeriod is the time to wait before deleting completed resources.
	// 0 implies the duration
	CompletedResourceGracePeriod time.Duration

	// Label selector to match resources against in order to determine
	// whether completed resources are eligible for deletion. The default
	// value is labels.Everything() which matches any resource.
	labelSelector labels.Selector

	// How long the controller waits to reprocess keys on certain events
	// (e.g. an object doesn't match the provided label selectors).
	RequeueInterval time.Duration

	// Check owner reference when deleting objects. By default, objects having owner references set won't be deleted.
	CheckOwner bool

	// UpdateLogTimeout is the time we provide for storing logs before aborting
	UpdateLogTimeout *time.Duration

	// DynamicReconcileTimeout is the time we provide for the dynamic reconciler to process an event
	DynamicReconcileTimeout *time.Duration

	// Whether to Store Events related to Taskrun and Pipelineruns
	StoreEvent bool

	// StoreDeadline is the time we provide for the PipelineRun and TaskRun resources
	// to be stored before aborting and clearing the finalizer in case of delete event
	StoreDeadline *time.Duration

	// FinalizerRequeueInterval is the duration after which finalizer reconciler
	// is scheduled to run for processing Runs not yet stored.
	FinalizerRequeueInterval time.Duration

	// ForwardBuffer is the time we provide for the TaskRun Logs to finish streaming
	// by a forwarder. Since there's no way to check if log has been streamed, we
	// always wait for this much amount of duration
	ForwardBuffer *time.Duration

	// Collect logs with timestamps
	LogsTimestamps bool

	// SummaryLabels are labels which should be part of the summary of the result
	SummaryLabels string

	// SummaryAnnotations are annotations which should be part of the summary of the result
	SummaryAnnotations string

	// DisableStoringIncompleteRuns disables the collection of incomplete Runs data
	DisableStoringIncompleteRuns bool

	// AllowedManagedByValues is the set of spec.managedBy values the watcher
	// will process. pipeline.ManagedBy ("tekton.dev/pipeline") is always
	// included. Runs with nil or empty managedBy are also always accepted.
	// This set must not be mutated after initialization to avoid data races.
	AllowedManagedByValues sets.Set[string]

	// RequiredAnnotations maps annotation keys to their requirements.
	// Every listed annotation must be present before the watcher clears its
	// finalizer. If the requirement specifies ExactMatch=true, the value must
	// also match. The stored annotation (results.tekton.dev/stored) is always
	// implicitly required and does not need to be listed. When nil/empty,
	// only the stored annotation is checked (existing behavior).
	RequiredAnnotations map[string]AnnotationRequirement
}

// AnnotationRequirement defines the condition an annotation must meet.
type AnnotationRequirement struct {
	// ExactMatch indicates if the annotation's value must exactly match Value.
	// If false, only the existence of the annotation key is checked.
	ExactMatch bool
	// Value is the expected value of the annotation when ExactMatch is true.
	Value string
}

// GetDisableAnnotationUpdate returns whether annotation updates should be
// disabled. This is safe to call for missing configs.
func (c *Config) GetDisableAnnotationUpdate() bool {
	if c == nil {
		return false
	}
	return c.DisableAnnotationUpdate
}

// GetCompletedResourceGracePeriod returns the grace period to wait for
// deleting Run objects.
// If value < 0, objects will be deleted immediately.
// If value = 0 (or not explicitly set), then objects will not be deleted.
// If value > 0, objects will be deleted with a grace period option of the
// duration.
func (c *Config) GetCompletedResourceGracePeriod() time.Duration {
	if c == nil {
		return 0
	}
	return c.CompletedResourceGracePeriod
}

// GetLabelSelector returns the label selector to match resources against in
// order to determine whether they're eligible for deletion. If no selector was
// configured via the SetLabelSelector method, returns a selector that always
// matches any resource.
func (c *Config) GetLabelSelector() labels.Selector {
	if c.labelSelector == nil {
		return labels.Everything()
	}
	return c.labelSelector
}

// SetLabelSelector sets a label selector to match resources against in order to
// determine whether they're eligible for deletion. The syntax obeys the same
// format accepted by list operations performed on the Kubernetes API server.
func (c *Config) SetLabelSelector(selector string) error {
	parsedSelector, err := labels.Parse(selector)
	if err != nil {
		return err
	}
	c.labelSelector = parsedSelector
	return nil
}

// AnnotationFlag implements flag.Value for a repeatable --required_annotation
// flag. Each invocation adds one entry to the map.
type AnnotationFlag map[string]AnnotationRequirement

// String returns a display representation used by flag --help.
func (f *AnnotationFlag) String() string {
	if f == nil || len(*f) == 0 {
		return ""
	}
	parts := make([]string, 0, len(*f))
	for k, req := range *f {
		if req.ExactMatch {
			parts = append(parts, k+"="+req.Value)
		} else {
			parts = append(parts, k)
		}
	}
	return strings.Join(parts, ", ")
}

// Set is called once per --required_annotation occurrence. It splits the
// value on the first "=" to extract the annotation key and expected value.
// If no "=" is present, it registers an existence-only requirement.
func (f *AnnotationFlag) Set(val string) error {
	val = strings.TrimSpace(val)
	if val == "" {
		return fmt.Errorf("invalid annotation: cannot be empty")
	}
	idx := strings.Index(val, "=")
	if idx == 0 {
		return fmt.Errorf(`invalid annotation %q, key cannot be empty`, val)
	}
	if *f == nil {
		*f = make(map[string]AnnotationRequirement)
	}
	if idx < 0 {
		(*f)[val] = AnnotationRequirement{ExactMatch: false}
	} else {
		(*f)[val[:idx]] = AnnotationRequirement{ExactMatch: true, Value: val[idx+1:]}
	}
	return nil
}

// AreRequiredAnnotationsReady checks whether every required annotation is
// present and meets its requirement (existence or exact value). Returns the
// key of the first unsatisfied annotation and false, or "" and true when all
// are satisfied.
func (c *Config) AreRequiredAnnotationsReady(annotations map[string]string) (missingKey string, ready bool) {
	if c == nil || len(c.RequiredAnnotations) == 0 {
		return "", true
	}
	for key, req := range c.RequiredAnnotations {
		val, exists := annotations[key]
		if !exists {
			return key, false
		}
		if req.ExactMatch && val != req.Value {
			return key, false
		}
	}
	return "", true
}
