/*
Copyright 2020 The Tekton Authors

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

// Package convert provides a method to convert v1beta1 API objects to Results
// API proto objects.
package convert

import (
	"encoding/json"
	"fmt"
	"github.com/tektoncd/results/pkg/apis/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/pkg/client/clientset/versioned/scheme"
	"github.com/tektoncd/pipeline/pkg/pod"
	rpb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"knative.dev/pkg/apis"
)

func ToProto(in runtime.Object) (*rpb.Any, error) {
	if in == nil {
		return nil, nil
	}

	b, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}

	return &rpb.Any{
		Type:  TypeName(in),
		Value: b,
	}, nil
}

func ToLogProto(in metav1.Object, recordName string) (*rpb.Any, error) {
	if in == nil {
		return nil, nil
	}

	trl := &v1alpha2.TaskRunLog{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: in.GetNamespace(),
			Name:      fmt.Sprintf("%s-log", in.GetName()),
		},
		Spec: v1alpha2.TaskRunLogSpec{
			Ref: v1alpha2.TaskRunRef{
				Namespace: in.GetNamespace(),
				Name:      in.GetName(),
			},
			RecordName: recordName,
			Type:       v1alpha2.FileLogType,
		},
	}
	trl.Default()
	b, err := json.Marshal(trl)
	if err != nil {
		return nil, err
	}
	return &rpb.Any{
		Type:  v1alpha2.TaskRunLogRecordType,
		Value: b,
	}, nil
}

// TypeName returns a string representation of type Object type.
// We do not know of any formalized spec for identifying objects across API
// versions. Standard GVK string formatting does not produce something that's
// payload friendly (i.e. includes spaces).
// To get around this we append API Version + Kind
// (e.g. tekton.dev/v1beta1.TaskRun).
func TypeName(in runtime.Object) string {
	gvk := in.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		// GVK not explicitly set in the object, fall back to scheme-based
		// lookup.
		var err error
		gvk, err = InferGVK(in)
		// Avoid returning back "." if the GVK doesn't contain any info.
		if err != nil || gvk.Empty() {
			return ""
		}
	}
	v, k := gvk.ToAPIVersionAndKind()
	return fmt.Sprintf("%s.%s", v, k)
}

// InferGVK infers the GroupVersionKind from the Object via schemes. Currently
// only the Tekton scheme is supported.
func InferGVK(o runtime.Object) (schema.GroupVersionKind, error) {
	gvks, _, err := scheme.Scheme.ObjectKinds(o)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}
	// This could potentially match a few different ones (not exactly sure
	// when this would happen), but generally shouldn't because we're using
	// the direct types from the Tekton package.
	if len(gvks) == 0 {
		return schema.GroupVersionKind{}, fmt.Errorf("could not determine GroupVersionKind for object")
	}
	return gvks[0], nil
}

// Status maps a Run condition to a general Record status.
func Status(ca apis.ConditionAccessor) rpb.RecordSummary_Status {
	c := ca.GetCondition(apis.ConditionSucceeded)
	if c == nil {
		return rpb.RecordSummary_UNKNOWN
	}

	switch v1beta1.TaskRunReason(c.Reason) {
	case v1beta1.TaskRunReasonSuccessful:
		return rpb.RecordSummary_SUCCESS
	case v1beta1.TaskRunReasonFailed:
		return rpb.RecordSummary_FAILURE
	case v1beta1.TaskRunReasonTimedOut:
		return rpb.RecordSummary_TIMEOUT
	case v1beta1.TaskRunReasonCancelled:
		return rpb.RecordSummary_CANCELLED
	case v1beta1.TaskRunReasonRunning, v1beta1.TaskRunReasonStarted:
		return rpb.RecordSummary_UNKNOWN
	}

	switch v1beta1.PipelineRunReason(c.Reason) {
	case v1beta1.PipelineRunReasonSuccessful, v1beta1.PipelineRunReasonCompleted:
		return rpb.RecordSummary_SUCCESS
	case v1beta1.PipelineRunReasonFailed:
		return rpb.RecordSummary_FAILURE
	case v1beta1.PipelineRunReasonTimedOut:
		return rpb.RecordSummary_TIMEOUT
	case v1beta1.PipelineRunReasonCancelled:
		return rpb.RecordSummary_CANCELLED
	case v1beta1.PipelineRunReasonRunning, v1beta1.PipelineRunReasonStarted, v1beta1.PipelineRunReasonPending, v1beta1.PipelineRunReasonStopping, v1beta1.PipelineRunReasonCancelledRunningFinally, v1beta1.PipelineRunReasonStoppedRunningFinally:
		return rpb.RecordSummary_UNKNOWN
	}

	switch c.Reason {
	case pod.ReasonCouldntGetTask, pod.ReasonFailedResolution, pod.ReasonFailedValidation, pod.ReasonExceededResourceQuota, pod.ReasonExceededNodeResources, pod.ReasonCreateContainerConfigError, pod.ReasonPodCreationFailed:
		return rpb.RecordSummary_FAILURE
	case pod.ReasonPending:
		return rpb.RecordSummary_UNKNOWN
	}
	return rpb.RecordSummary_UNKNOWN
}
