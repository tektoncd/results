package main

import (
	"time"

	"github.com/golang/protobuf/ptypes"
	durpb "github.com/golang/protobuf/ptypes/duration"
	tspb "github.com/golang/protobuf/ptypes/timestamp"
	v1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	pb "github.com/tektoncd/results/proto/pipeline/v1/pipeline_go_proto"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

// This file just contains test data to simplify the content in main_test.go.

var (
	create = metav1.Time{Time: time.Unix(1, 0)}
	delete = metav1.Time{Time: time.Unix(2, 0)}
	start  = metav1.Time{Time: time.Unix(3, 0)}
	finish = metav1.Time{Time: time.Unix(4, 0)}

	taskrun = &v1.TaskRun{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "tekton.dev/v1",
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
		Spec: v1.TaskRunSpec{
			Timeout: &metav1.Duration{Duration: time.Hour},
			TaskSpec: &v1.TaskSpec{
				Steps: []v1.Step{{
					Script:     "script",
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
				}, {
					Name: "step2",
				}},
				Sidecars: []v1.Sidecar{{
					Name: "sidecar1",
				}, {
					Name: "sidecar2",
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
		Status: v1.TaskRunStatus{
			Status: duckv1.Status{
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
			TaskRunStatusFields: v1.TaskRunStatusFields{
				PodName:        "podname",
				StartTime:      &start,
				CompletionTime: &finish,
				Steps: []v1.StepState{{
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
					Name:      "name",
					Container: "containername",
					ImageID:   "imageid",
				}, {
					Name: "another state",
				}},
			},
		},
	}
	taskrunpb = &pb.TaskRun{
		ApiVersion: "tekton.dev/v1",
		Kind:       "TaskRun",
		Metadata: &pb.ObjectMeta{
			Name:              "name",
			GenerateName:      "generate-name",
			Namespace:         "namespace",
			Uid:               "uid",
			Generation:        12345,
			CreationTimestamp: timestamp(&create),
			DeletionTimestamp: timestamp(&delete),
			Labels: map[string]string{
				"label-one": "one",
				"label-two": "two",
			},
			Annotations: map[string]string{
				"annotation-one": "one",
				"annotation-two": "two",
			},
		},
		Spec: &pb.TaskRunSpec{
			Timeout: &durpb.Duration{Seconds: 3600},
			TaskSpec: &pb.TaskSpec{
				Steps: []*pb.Step{{
					Script:     "script",
					Name:       "name",
					Image:      "image",
					Command:    []string{"cmd1", "cmd2"},
					Args:       []string{"arg1", "arg2"},
					WorkingDir: "workingdir",
					Env: []*pb.EnvVar{{
						Name:  "env1",
						Value: "ENV1",
					}, {
						Name:  "env2",
						Value: "ENV2",
					}},
					VolumeMounts: []*pb.VolumeMount{{
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
				}, {
					Name: "step2",
				}},
				Sidecars: []*pb.Step{{
					Name: "sidecar1",
				}, {
					Name: "sidecar2",
				}},
				Volumes: []*pb.Volume{{
					Name:   "volname1",
					Source: &pb.Volume_EmptyDir{EmptyDir: &pb.EmptyDir{}},
				}, {
					Name:   "volname2",
					Source: &pb.Volume_EmptyDir{EmptyDir: &pb.EmptyDir{}},
				}},
			},
		},
		Status: &pb.TaskRunStatus{
			Conditions: []*pb.Condition{{
				Type:               "type",
				Status:             "status",
				Severity:           "omgbad",
				LastTransitionTime: timestamp(&finish),
				Reason:             "reason",
				Message:            "message",
			}, {
				Type: "another condition",
			}},
			ObservedGeneration: 23456,
			PodName:            "podname",
			StartTime:          timestamp(&start),
			CompletionTime:     timestamp(&finish),
			Steps: []*pb.StepState{{
				Status: &pb.StepState_Terminated{Terminated: &pb.ContainerStateTerminated{
					ExitCode:    123,
					Signal:      456,
					Reason:      "reason",
					Message:     "message",
					StartedAt:   timestamp(&start),
					FinishedAt:  timestamp(&finish),
					ContainerId: "containerid",
				}},
				Name:          "name",
				ContainerName: "containername",
				ImageId:       "imageid",
			}, {
				Name: "another state",
			}},
		},
	}
	pipelinerun = &v1.PipelineRun{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "tekton.dev/v1",
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
		Spec: v1.PipelineRunSpec{
			Timeouts: &v1.TimeoutFields{Pipeline: &metav1.Duration{Duration: time.Hour}},
			PipelineSpec: &v1.PipelineSpec{
				Tasks: []v1.PipelineTask{{
					Name: "ptask",
					TaskRef: &v1.TaskRef{
						Name:       "ptask",
						Kind:       "kind",
						APIVersion: "api_version",
					},
					TaskSpec: &v1.EmbeddedTask{
						Metadata: v1.PipelineTaskMetadata{
							Labels: map[string]string{
								"label-one": "one",
							},
							Annotations: map[string]string{
								"ann-one": "one",
							},
						},
						TaskSpec: v1.TaskSpec{
							Steps: []v1.Step{{
								Script:     "script",
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
							}},
							Sidecars: []v1.Sidecar{{}},
							Volumes: []corev1.Volume{{
								Name:         "volname1",
								VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
							}},
						},
					},
					Timeout: &metav1.Duration{Duration: time.Hour},
				}},
				Results: []v1.PipelineResult{{
					Name:        "result",
					Description: "desc",
					Value:       *v1.NewStructuredValues("value"),
				}},
				Finally: []v1.PipelineTask{{}},
			},
		},
		Status: v1.PipelineRunStatus{
			Status: duckv1.Status{
				ObservedGeneration: 12345,
				Conditions:         []apis.Condition{{}},
				Annotations: map[string]string{
					"ann-one": "one",
				},
			},
			PipelineRunStatusFields: v1.PipelineRunStatusFields{
				ChildReferences: []v1.ChildStatusReference{{
					Name: "pipelineTaskName",
				}},
				PipelineSpec: &v1.PipelineSpec{},
			},
		},
	}
	pipelinerunpb = &pb.PipelineRun{
		ApiVersion: "tekton.dev/v1",
		Kind:       "PipelineRun",
		Spec: &pb.PipelineRunSpec{
			Timeout: &durpb.Duration{Seconds: 3600},
			PipelineSpec: &pb.PipelineSpec{
				Tasks: []*pb.PipelineTask{{
					Name: "ptask",
					TaskRef: &pb.TaskRef{
						Name:       "ptask",
						Kind:       "kind",
						ApiVersion: "api_version",
					},
					TaskSpec: &pb.EmbeddedTask{
						Metadata: &pb.PipelineTaskMetadata{
							Labels: map[string]string{
								"label-one": "one",
							},
							Annotations: map[string]string{
								"ann-one": "one",
							},
						},
						Steps: []*pb.Step{{
							Script:     "script",
							Name:       "name",
							Image:      "image",
							Command:    []string{"cmd1", "cmd2"},
							Args:       []string{"arg1", "arg2"},
							WorkingDir: "workingdir",
							Env: []*pb.EnvVar{{
								Name:  "env1",
								Value: "ENV1",
							}, {
								Name:  "env2",
								Value: "ENV2",
							}},
							VolumeMounts: []*pb.VolumeMount{{
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
						}},
						Sidecars: []*pb.Step{{}},
						Volumes: []*pb.Volume{{
							Name:   "volname1",
							Source: &pb.Volume_EmptyDir{EmptyDir: &pb.EmptyDir{}},
						}},
					},
					Timeout: &durpb.Duration{Seconds: 3600},
				}},
				Results: []*pb.PipelineResult{{
					Name:        "result",
					Description: "desc",
					Value:       "value",
				}},
				Finally: []*pb.PipelineTask{{}},
			},
		},
		Status: &pb.PipelineRunStatus{
			ObservedGeneration: 12345,
			Conditions:         []*pb.Condition{{}},
			Annotations: map[string]string{
				"ann-one": "one",
			},
			TaskRuns: map[string]*pb.PipelineRunTaskRunStatus{
				"task": {
					PipelineTaskName: "pipelineTaskName",
					Status:           &pb.TaskRunStatus{},
				},
			},
			PipelineSpec: &pb.PipelineSpec{},
		},
		Metadata: &pb.ObjectMeta{
			Name:              "test-pipeline",
			GenerateName:      "test-pipeline-",
			Namespace:         "namespace",
			Uid:               "uid",
			Generation:        12345,
			CreationTimestamp: timestamp(&create),
			DeletionTimestamp: timestamp(&delete),
			Labels: map[string]string{
				"label-one": "one",
			},
			Annotations: map[string]string{
				"ann-one": "one",
			},
		},
	}
)

func timestamp(t *metav1.Time) *tspb.Timestamp {
	if t == nil {
		return nil
	}
	if t.Time.IsZero() {
		return nil
	}
	p, err := ptypes.TimestampProto(t.Time.Truncate(time.Second))
	if err != nil {
		panic(err.Error())
	}
	return p
}
