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
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (

	// Result identifier.
	Result = "results.tekton.dev/result"

	// Record identifier.
	Record = "results.tekton.dev/record"

	Log = "results.tekton.dev/log"

	// Integrators should add this annotation to objects in order to store
	// arbitrary keys/values into the Result.Annotations field.
	ResultAnnotations = "results.tekton.dev/resultAnnotations"

	// Integrators should add this annotation to objects in order to store
	// arbitrary keys/values into the Result.Summary.Annotations field.
	RecordSummaryAnnotations = "results.tekton.dev/recordSummaryAnnotations"

	// Annotation that signals to the controller that a given child object
	// (e.g. TaskRun owned by a PipelineRun) is done and up to date in the
	// API server and therefore, ready to be garbage collected.
	ChildReadyForDeletion = "results.tekton.dev/childReadyForDeletion"
)

type Annotation struct {
	Name  string
	Value string
}

type mergePatch struct {
	Metadata metadata `json:"metadata"`
}

type metadata struct {
	Annotations map[string]string `json:"annotations"`
}

// Patch creates a jsonpatch path used for adding result / record identifiers as
// well as other internal annotations to an object's annotations field.
func Patch(object metav1.Object, annotations ...Annotation) ([]byte, error) {
	data := mergePatch{
		Metadata: metadata{
			Annotations: map[string]string{},
		},
	}

	for _, annotation := range annotations {
		if len(annotation.Value) != 0 {
			data.Metadata.Annotations[annotation.Name] = annotation.Value
		}
	}

	if isChildAndDone(object) {
		data.Metadata.Annotations[ChildReadyForDeletion] = "true"
	}
	return json.Marshal(data)
}

// isChildAndDone returns true if the object in question is a child resource
// (i.e. has owner references) and it's done, therefore eligible to be patched
// with the results.tekton.dev/childReadyForDeletion annotation.
func isChildAndDone(objecct metav1.Object) bool {
	if len(objecct.GetOwnerReferences()) == 0 {
		return false
	}

	doneObj, ok := objecct.(interface{ IsDone() bool })
	if !ok {
		return false
	}
	return doneObj.IsDone()
}

// IsPatched returns true if the object in question contains all relevant
// annotations or false otherwise.
func IsPatched(object metav1.Object, annotations ...Annotation) bool {
	objAnnotations := object.GetAnnotations()
	if isChildAndDone(object) {
		if _, found := objAnnotations[ChildReadyForDeletion]; !found {
			return false
		}
	}

	for _, annotation := range annotations {
		if objAnnotations[annotation.Name] != annotation.Value {
			return false
		}
	}

	return true
}
