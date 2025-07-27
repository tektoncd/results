// Copyright 2020 The Tekton Authors
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

package annotation

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/tektoncd/results/pkg/watcher/reconciler/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/logging"
)

const (
	// Result identifier.
	Result = "results.tekton.dev/result"

	// Record identifier.
	Record = "results.tekton.dev/record"

	// Log identifier.
	Log = "results.tekton.dev/log"

	// EventList identifier.
	EventList = "results.tekton.dev/eventlist"

	// Stored is an annotation that signals to the controller that a given object
	// has been stored by the Results API.
	Stored = "results.tekton.dev/stored"

	// ResultAnnotations is an annotation that integrators should add to objects in order to store
	// arbitrary keys/values into the Result.Annotations field.
	ResultAnnotations = "results.tekton.dev/resultAnnotations"

	// RecordSummaryAnnotations is an annotation that integrators should add to objects
	// in order to store arbitrary keys/values into the Result.Summary.Annotations field.
	// This allows for additional information to be associated with the summary of a record.
	RecordSummaryAnnotations = "results.tekton.dev/recordSummaryAnnotations"

	// ChildReadyForDeletion is an annotation that signals to the controller that a given child object
	// (e.g. TaskRun owned by a PipelineRun) is done and up to date in the
	// API server and therefore, ready to be garbage collected.
	ChildReadyForDeletion = "results.tekton.dev/childReadyForDeletion"

	// FieldManager identifier to be used with Server-Side Apply patches
	fieldManager = "tekton-results-watcher"
)

// ManagedAnnotations contains all annotation keys managed by the results watcher.
// When adding new annotations above, add them to this slice as well.
var ManagedAnnotations = []string{
	Result,
	Record,
	Log,
	EventList,
	Stored,
	ResultAnnotations,
	RecordSummaryAnnotations,
	ChildReadyForDeletion,
}

// Annotation is wrapper for Kubernetes resource annotations stored in the metadata.
type Annotation struct {
	Name  string
	Value string
}

// Server-side apply patch structure
type applyPatch struct {
	APIVersion string   `json:"apiVersion"`
	Kind       string   `json:"kind"`
	Metadata   metadata `json:"metadata"`
}

type metadata struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Annotations map[string]string `json:"annotations"`
}

// Patch builds and applies a patch with the given annotations to the object using the provided object client.
func Patch(
	ctx context.Context,
	object metav1.Object,
	objectClient client.ObjectClient,
	annotations ...Annotation,
) error {

	logger := logging.FromContext(ctx)

	// Get the API version and kind from the object
	var apiVersion, kind string
	if runtimeObj, ok := object.(runtime.Object); ok {
		if gvk := runtimeObj.GetObjectKind().GroupVersionKind(); !gvk.Empty() {
			kind = gvk.Kind
			apiVersion = gvk.GroupVersion().String()
		}
	}
	// If we couldn't determine the kind or apiVersion, fail
	if kind == "" || apiVersion == "" {
		logger.Errorf("could not determine apiVersion and kind from object %s/%s", object.GetNamespace(), object.GetName())
		return fmt.Errorf("could not determine apiVersion and kind from object %s/%s", object.GetNamespace(), object.GetName())
	}

	if IsPatched(object, annotations...) {
		logger.Debugf("Skipping CRD annotation patch: annotations are already set ObjectName: %s", object.GetName())
		return nil
	}

	data := applyPatch{
		APIVersion: apiVersion,
		Kind:       kind,
		Metadata: metadata{
			Name:        object.GetName(),
			Namespace:   object.GetNamespace(),
			Annotations: map[string]string{},
		},
	}

	// Copy existing managed annotations from the object
	// Only include annotations that we manage (results.tekton.dev/* annotations)
	// to avoid conflicts with other controllers using server-side apply
	currentAnnotations := object.GetAnnotations()
	for key, value := range currentAnnotations {
		if slices.Contains(ManagedAnnotations, key) {
			data.Metadata.Annotations[key] = value
		}
	}

	// Add/overwrite with new annotations
	for _, annotation := range annotations {
		if len(annotation.Value) != 0 {
			data.Metadata.Annotations[annotation.Name] = annotation.Value
		}
	}
	patch, err := json.Marshal(data)
	if err != nil {
		return err
	}

	patchOptions := metav1.PatchOptions{
		FieldManager: fieldManager,
		Force:        &[]bool{true}[0], // Force the update if conflict as only we should be updating those annotations
	}
	err = objectClient.Patch(ctx, object.GetName(), types.ApplyPatchType, patch, patchOptions)

	// After successful patch, update in-memory object
	if err == nil {
		currentAnnotations := object.GetAnnotations()
		if currentAnnotations == nil {
			currentAnnotations = make(map[string]string)
		}
		for _, ann := range annotations {
			currentAnnotations[ann.Name] = ann.Value
		}
		object.SetAnnotations(currentAnnotations)
	}

	return err
}

// IsPatched returns true if the object in question contains all relevant
// annotations or false otherwise.
func IsPatched(object metav1.Object, annotations ...Annotation) bool {
	objAnnotations := object.GetAnnotations()
	for _, annotation := range annotations {
		if objAnnotations[annotation.Name] != annotation.Value {
			return false
		}
	}
	return true
}
