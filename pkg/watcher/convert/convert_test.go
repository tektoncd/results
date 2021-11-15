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
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	rpb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
		want     proto.Message
		wantType string
	}{
		{
			in: taskrun,
			//want:     taskrunpb,
			wantType: "tekton.dev/v1beta1.TaskRun",
		},
		{
			in: pipelinerun,
			//want:     pipelinerunpb,
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
