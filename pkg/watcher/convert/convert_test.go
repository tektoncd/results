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

package convert

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/pkg/pod"
	rpb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/protobuf/testing/protocmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"knative.dev/pkg/apis"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
)

var (
	create = metav1.Time{Time: time.Unix(1, 0)}
	delete = metav1.Time{Time: time.Unix(2, 0)}
	start  = metav1.Time{Time: time.Unix(3, 0)}
	finish = metav1.Time{Time: time.Unix(4, 0)}

	taskrun = &v1beta1.TaskRun{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "tekton.dev/v1beta1",
			Kind:       "TaskRun",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "name",
			GenerateName:      "generate-name",
			Namespace:         "namespace",
			UID:               "uid",
			Generation:        12345,
			CreationTimestamp: create,
			DeletionTimestamp: &delete,
			Labels: map[string]string{
				"label-one": "one",
				"label-two": "two",
			},
			Annotations: map[string]string{
				"annotation-one": "one",
				"annotation-two": "two",
			},
		},
		Spec: v1beta1.TaskRunSpec{
			Timeout: &metav1.Duration{Duration: time.Hour},
			TaskSpec: &v1beta1.TaskSpec{
				Steps: []v1beta1.Step{{
					Script: "script",
					Container: corev1.Container{
						Name:       "name",
						Image:      "image",
						Command:    []string{"cmd1", "cmd2"},
						Args:       []string{"arg1", "arg2"},
						WorkingDir: "workingdir",
						Env: []corev1.EnvVar{{
							Name:  "env1",
							Value: "ENV1",
						}, {
							Name:  "env2",
							Value: "ENV2",
						}},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "vm1",
							MountPath: "path1",
							ReadOnly:  false,
							SubPath:   "subpath1",
						}, {
							Name:      "vm2",
							MountPath: "path2",
							ReadOnly:  true,
							SubPath:   "subpath2",
						}},
					},
				}, {
					Container: corev1.Container{Name: "step2"},
				}},
				Sidecars: []v1beta1.Sidecar{{
					Container: corev1.Container{Name: "sidecar1"},
				}, {
					Container: corev1.Container{Name: "sidecar2"},
				}},
				Volumes: []corev1.Volume{{
					Name:         "volname1",
					VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
				}, {
					Name:         "volname2",
					VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
				}},
			},
		},
		Status: v1beta1.TaskRunStatus{
			Status: duckv1beta1.Status{
				ObservedGeneration: 23456,
				Conditions: []apis.Condition{{
					Type:               "type",
					Status:             "status",
					Severity:           "omgbad",
					LastTransitionTime: apis.VolatileTime{Inner: finish},
					Reason:             "reason",
					Message:            "message",
				}, {
					Type: "another condition",
				}},
			},
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				PodName:        "podname",
				StartTime:      &start,
				CompletionTime: &finish,
				Steps: []v1beta1.StepState{{
					ContainerState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode:    123,
							Signal:      456,
							Reason:      "reason",
							Message:     "message",
							StartedAt:   start,
							FinishedAt:  finish,
							ContainerID: "containerid",
						},
					},
					Name:          "name",
					ContainerName: "containername",
					ImageID:       "imageid",
				}, {
					Name: "another state",
				}},
			},
		},
	}

	pipelinerun = &v1beta1.PipelineRun{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "tekton.dev/v1beta1",
			Kind:       "PipelineRun",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-pipeline",
			GenerateName:      "test-pipeline-",
			Namespace:         "namespace",
			UID:               "uid",
			Generation:        12345,
			CreationTimestamp: create,
			DeletionTimestamp: &delete,
			Labels: map[string]string{
				"label-one": "one",
			},
			Annotations: map[string]string{
				"ann-one": "one",
			},
		},
		Spec: v1beta1.PipelineRunSpec{
			Timeout: &metav1.Duration{Duration: time.Hour},
			PipelineSpec: &v1beta1.PipelineSpec{
				Tasks: []v1beta1.PipelineTask{{
					Name: "ptask",
					TaskRef: &v1beta1.TaskRef{
						Name:       "ptask",
						Kind:       "kind",
						APIVersion: "api_version",
					},
					TaskSpec: &v1beta1.EmbeddedTask{
						Metadata: v1beta1.PipelineTaskMetadata{
							Labels: map[string]string{
								"label-one": "one",
							},
							Annotations: map[string]string{
								"ann-one": "one",
							},
						},
						TaskSpec: v1beta1.TaskSpec{
							Steps: []v1beta1.Step{{
								Script: "script",
								Container: corev1.Container{
									Name:       "name",
									Image:      "image",
									Command:    []string{"cmd1", "cmd2"},
									Args:       []string{"arg1", "arg2"},
									WorkingDir: "workingdir",
									Env: []corev1.EnvVar{{
										Name:  "env1",
										Value: "ENV1",
									}, {
										Name:  "env2",
										Value: "ENV2",
									}},
									VolumeMounts: []corev1.VolumeMount{{
										Name:      "vm1",
										MountPath: "path1",
										ReadOnly:  false,
										SubPath:   "subpath1",
									}, {
										Name:      "vm2",
										MountPath: "path2",
										ReadOnly:  true,
										SubPath:   "subpath2",
									}},
								},
							}},
							Sidecars: []v1beta1.Sidecar{{}},
							Volumes: []corev1.Volume{{
								Name:         "volname1",
								VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
							}},
						},
					},
					Timeout: &metav1.Duration{Duration: time.Hour},
				}},
				Results: []v1beta1.PipelineResult{{
					Name:        "result",
					Description: "desc",
					Value:       "value",
				}},
				Finally: []v1beta1.PipelineTask{{}},
			},
		},
		Status: v1beta1.PipelineRunStatus{
			Status: duckv1beta1.Status{
				ObservedGeneration: 12345,
				Conditions:         []apis.Condition{{}},
				Annotations: map[string]string{
					"ann-one": "one",
				},
			},
			PipelineRunStatusFields: v1beta1.PipelineRunStatusFields{
				TaskRuns: map[string]*v1beta1.PipelineRunTaskRunStatus{
					"task": {
						PipelineTaskName: "pipelineTaskName",
						Status:           &v1beta1.TaskRunStatus{},
					},
				},
				PipelineSpec: &v1beta1.PipelineSpec{},
			},
		},
	}
)

func TestToProto(t *testing.T) {
	for _, tc := range []struct {
		in       runtime.Object
		wantType string
	}{
		{
			in:       taskrun,
			wantType: "tekton.dev/v1beta1.TaskRun",
		},
		{
			in:       pipelinerun,
			wantType: "tekton.dev/v1beta1.PipelineRun",
		},
	} {
		t.Run(fmt.Sprintf("%T", tc.wantType), func(t *testing.T) {
			// Generate unstructured variant to make sure we can handle both types.
			u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(tc.in)
			if err != nil {
				t.Fatalf("ToUnstructured: %v", err)
			}

			for _, o := range []runtime.Object{
				tc.in,
				&unstructured.Unstructured{Object: u},
			} {
				t.Run(fmt.Sprintf("%T", o), func(t *testing.T) {
					got, err := ToProto(o)
					if err != nil {
						t.Fatalf("ToProto: %v", err)
					}

					want := &rpb.Any{
						Type:  tc.wantType,
						Value: toJSON(o),
					}
					if d := cmp.Diff(want, got, protocmp.Transform()); d != "" {
						t.Errorf("Diff(-want,+got): %s", d)
					}
				})
			}
		})
	}

	t.Run("nil", func(t *testing.T) {
		got, err := ToProto(nil)
		if err != nil {
			t.Fatalf("ToProto: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

}

func toJSON(i interface{}) []byte {
	b, err := json.Marshal(i)
	if err != nil {
		panic(fmt.Sprintf("error marshalling json: %v", err))
	}
	return b
}

func TestTypeName(t *testing.T) {
	for _, tc := range []struct {
		i    runtime.Object
		want string
	}{
		{
			i:    &v1beta1.TaskRun{},
			want: "tekton.dev/v1beta1.TaskRun",
		},
		{
			i:    &v1beta1.PipelineRun{},
			want: "tekton.dev/v1beta1.PipelineRun",
		},
		{
			i:    &v1alpha1.TaskRun{},
			want: "tekton.dev/v1alpha1.TaskRun",
		},
		{
			// This shouldn't really happen, but serves as an example of what
			// happens if clients manually override the TypeMeta in the object.
			i: &v1beta1.TaskRun{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "foo",
					Kind:       "bar",
				},
			},
			want: "foo.bar",
		},
	} {
		t.Run(tc.want, func(t *testing.T) {
			if got := TypeName(tc.i); got != tc.want {
				t.Errorf("want %s, got %s", tc.want, got)
			}
		})
	}
}

func TestInferGVK(t *testing.T) {
	for _, tc := range []struct {
		o       runtime.Object
		want    schema.GroupVersionKind
		wantErr bool
	}{
		{
			o:    &v1beta1.TaskRun{},
			want: schema.FromAPIVersionAndKind("tekton.dev/v1beta1", "TaskRun"),
		},
		{
			o:    &v1beta1.PipelineRun{},
			want: schema.FromAPIVersionAndKind("tekton.dev/v1beta1", "PipelineRun"),
		},
		{
			o:    &v1alpha1.PipelineRun{},
			want: schema.FromAPIVersionAndKind("tekton.dev/v1alpha1", "PipelineRun"),
		},
		// We only load in the Tekton type scheme, so other Objects won't be recognized.
		{
			o:       &unstructured.Unstructured{},
			wantErr: true,
		},
	} {
		got, err := InferGVK(tc.o)
		if err != nil {
			if tc.wantErr {
				return
			}
			t.Fatal(err)
		}
		if !cmp.Equal(tc.want, got) {
			t.Errorf("want %s, got %s", tc.want.String(), got.String())
		}
	}
}

func TestStatus(t *testing.T) {
	for _, tc := range []struct {
		cond *apis.Condition
		want rpb.RecordSummary_Status
	}{
		// We are not testing an exhaustive list of statuses here,
		// since there's no way to iterate through the set of all possible
		// statuses.
		// Mapping every case 1:1 would effectively be a
		// reimplementation of the code we are trying to test, which isn't
		// particularly useful. Instead, we focus on testing 1 status for
		// each type + any edge cases.
		{
			cond: &apis.Condition{
				Type:    apis.ConditionSucceeded,
				Reason:  string(v1beta1.TaskRunReasonSuccessful),
				Message: "TaskRun Success",
			},
			want: rpb.RecordSummary_SUCCESS,
		},
		{
			cond: &apis.Condition{
				Type:    apis.ConditionSucceeded,
				Reason:  string(v1beta1.PipelineRunReasonTimedOut),
				Message: "PipelineRun Timeout",
			},
			want: rpb.RecordSummary_TIMEOUT,
		},
		{
			cond: &apis.Condition{
				Type:    apis.ConditionSucceeded,
				Reason:  string(pod.ReasonCouldntGetTask),
				Message: "Pod Failure",
			},
			want: rpb.RecordSummary_FAILURE,
		},
		{
			// Statuses only work on ConditionSucceeded, since this is what
			// tells us the final state of the Run.
			cond: &apis.Condition{
				Type:    apis.ConditionReady,
				Reason:  string(v1beta1.TaskRunReasonSuccessful),
				Message: "Ready Condition",
			},
			want: rpb.RecordSummary_UNKNOWN,
		},
		{
			cond: &apis.Condition{
				Type:    apis.ConditionSucceeded,
				Reason:  "foo",
				Message: "Unknown reason",
			},
			want: rpb.RecordSummary_UNKNOWN,
		},
		{
			cond: nil,
			want: rpb.RecordSummary_UNKNOWN,
		},
	} {
		t.Run(tc.cond.GetMessage(), func(t *testing.T) {
			got := Status(newConditionAccessor(tc.cond))
			if tc.want != got {
				t.Errorf("want %v, got %v", tc.want, got)
			}
		})
	}
}

// conditionAccessor is a simple impl of the apis.ConditionAccessor interface.
type conditionAccessor struct {
	m map[apis.ConditionType]*apis.Condition
}

func newConditionAccessor(conds ...*apis.Condition) conditionAccessor {
	m := map[apis.ConditionType]*apis.Condition{}
	for _, c := range conds {
		if c == nil {
			continue
		}
		m[c.Type] = c
	}
	return conditionAccessor{m: m}
}

func (ca conditionAccessor) GetCondition(t apis.ConditionType) *apis.Condition {
	return ca.m[t]
}
