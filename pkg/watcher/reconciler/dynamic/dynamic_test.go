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

package dynamic

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	pipelineclient "github.com/tektoncd/pipeline/pkg/client/injection/client"
	rtesting "github.com/tektoncd/pipeline/pkg/reconciler/testing"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/result"
	"github.com/tektoncd/results/pkg/internal/test"
	"github.com/tektoncd/results/pkg/watcher/reconciler"
	"github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	dynamicclient "k8s.io/client-go/dynamic/fake"
	"knative.dev/pkg/apis"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
	"knative.dev/pkg/controller"

	// Needed for informer injection.
	_ "github.com/tektoncd/pipeline/test"
)

type env struct {
	ctx     context.Context
	ctrl    *controller.Impl
	results pb.ResultsClient
	dynamic *dynamicclient.FakeDynamicClient
}

var (
	taskrun = &v1beta1.TaskRun{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "tekton.dev/v1beta1",
			Kind:       "TaskRun",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "taskrun",
			Namespace:   "ns",
			Annotations: map[string]string{"demo": "demo"},
			UID:         "12345",
		},
		Spec: v1beta1.TaskRunSpec{
			TaskSpec: &v1beta1.TaskSpec{
				Steps: []v1beta1.Step{{
					Script: "echo hello world!",
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
			Name:        "pipelinerun",
			Namespace:   "ns",
			Annotations: map[string]string{"demo": "demo"},
			UID:         "12345",
		},
		Status: v1beta1.PipelineRunStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{
					apis.Condition{
						Type:   apis.ConditionSucceeded,
						Status: corev1.ConditionTrue,
					},
				},
			},
			PipelineRunStatusFields: v1beta1.PipelineRunStatusFields{},
		},
		Spec: v1beta1.PipelineRunSpec{
			PipelineSpec: &v1beta1.PipelineSpec{
				Tasks: []v1beta1.PipelineTask{{
					Name: "task",
					TaskSpec: &v1beta1.EmbeddedTask{
						TaskSpec: v1beta1.TaskSpec{
							Steps: []v1beta1.Step{{
								Script: "echo hello world!",
							}},
						},
					},
				}},
			},
		},
	}
)

func TestReconcile_TaskRun(t *testing.T) {
	// Configures fake tekton clients + informers.
	ctx, _ := rtesting.SetupFakeContext(t)
	results := test.NewResultsClient(t)

	fakeclock := clockwork.NewFakeClockAt(time.Now())
	clock = fakeclock

	trclient := &TaskRunClient{TaskRunInterface: pipelineclient.Get(ctx).TektonV1beta1().TaskRuns(taskrun.GetNamespace())}
	if _, err := trclient.Create(ctx, taskrun, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	cfg := &reconciler.Config{
		DisableAnnotationUpdate: true,
	}

	r := NewDynamicReconciler(results, trclient, cfg)
	if err := r.Reconcile(ctx, taskrun); err != nil {
		t.Fatal(err)
	}

	taskrun.Status.InitializeConditions()

	t.Run("DisabledAnnotations", func(t *testing.T) {
		resultName := result.FormatName(taskrun.GetNamespace(), string(taskrun.GetUID()))
		if _, err := results.GetResult(ctx, &pb.GetResultRequest{Name: resultName}); err != nil {
			t.Fatalf("GetResult: %v", err)
		}
		recordName := record.FormatName(resultName, string(taskrun.GetUID()))
		if _, err := results.GetRecord(ctx, &pb.GetRecordRequest{Name: recordName}); err != nil {
			t.Fatalf("GetRecord: %v", err)
		}
	})

	// Enable Annotation Updates, re-reconcile
	cfg.DisableAnnotationUpdate = false
	if err := r.Reconcile(ctx, taskrun); err != nil {
		t.Fatal(err)
	}

	tr, err := trclient.Get(ctx, taskrun.GetName(), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{annotation.Result, annotation.Record} {
		if _, ok := tr.GetAnnotations()[a]; !ok {
			t.Errorf("annotation %s missing", a)
		}
	}

	if _, err := results.GetResult(ctx, &pb.GetResultRequest{Name: tr.GetAnnotations()[annotation.Result]}); err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if _, err := results.GetRecord(ctx, &pb.GetRecordRequest{Name: tr.GetAnnotations()[annotation.Record]}); err != nil {
		t.Fatalf("GetRecord: %v", err)
	}

	t.Run("delete object once grace period elapses", func(t *testing.T) {
		// Enable object deletion, re-reconcile
		cfg.CompletedResourceGracePeriod = 1 * time.Second

		isRequeueKey := func(err error) bool {
			ok, _ := controller.IsRequeueKey(err)
			return ok
		}

		// Simulate a successful TaskRun. The next test case will make
		// sure that failed objects can be deleted as well.
		taskrun.Status.SetCondition(&apis.Condition{
			Type:   apis.ConditionSucceeded,
			Status: corev1.ConditionTrue,
			Reason: string(v1beta1.TaskRunReasonSuccessful),
		})

		// The controller must requeue the object since the
		// CompletionTime field is nil.
		if err := r.Reconcile(ctx, taskrun); !isRequeueKey(err) {
			t.Fatalf("Want a controller.RequeueKey error, but got %v", err)
		}

		// Set the completion time and reconcile again.
		taskrun.Status.CompletionTime = &metav1.Time{Time: fakeclock.Now()}
		// The controller must requeue the object since the grace period
		// hasn't elapsed yet.
		if err := r.Reconcile(ctx, taskrun); !isRequeueKey(err) {
			t.Fatalf("Want a controller.RequeueKey error, but got %v", err)
		}

		// Advance the clock to force the grace period to elapse. The
		// TaskRun must be processed and deleted from the cluster.
		fakeclock.Advance(2 * time.Second)
		if err := r.Reconcile(ctx, taskrun); err != nil {
			t.Fatal(err)
		}

		_, err := trclient.Get(ctx, taskrun.GetName(), metav1.GetOptions{})
		if !apierrors.IsNotFound(err) {
			t.Fatalf("wanted NotFound, got %v", err)
		}
	})

	t.Run("delete failed runs", func(t *testing.T) {
		// Recreate the object to retest the deletion
		if _, err := trclient.Create(ctx, taskrun, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}

		// Simulate a failed run, set the completion time and advance
		// the clock to make this test case more independent from the
		// previous one.
		taskrun.Status.MarkResourceFailed(v1beta1.TaskRunReasonFailed, errors.New("Failed"))
		taskrun.Status.CompletionTime = &metav1.Time{Time: fakeclock.Now()}
		fakeclock.Advance(2 * time.Second)

		if err := r.Reconcile(ctx, taskrun); err != nil {
			t.Fatal(err)
		}

		// Make sure that the resource no longer exists
		if _, err := trclient.Get(ctx, taskrun.GetName(), metav1.GetOptions{}); !apierrors.IsNotFound(err) {
			t.Fatalf("Want NotFound, but got %v", err)
		}
	})
}

// This is a simpler test than TaskRun, since most of this behavior is
// generalized within the Dynamic clients - the primary thing we're testing
// here is that the Pipeline clients can be wired up properly.
func TestReconcile_PipelineRun(t *testing.T) {
	// Configures fake tekton clients + informers.
	ctx, _ := rtesting.SetupFakeContext(t)
	results := test.NewResultsClient(t)

	fakeclock := clockwork.NewFakeClockAt(time.Now())
	clock = fakeclock

	prclient := &PipelineRunClient{PipelineRunInterface: pipelineclient.Get(ctx).TektonV1beta1().PipelineRuns(pipelinerun.GetNamespace())}
	if _, err := prclient.Create(ctx, pipelinerun, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	r := NewDynamicReconciler(results, prclient, nil)
	if err := r.Reconcile(ctx, pipelinerun); err != nil {
		t.Fatal(err)
	}

	pr, err := prclient.Get(ctx, pipelinerun.GetName(), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{annotation.Result, annotation.Record} {
		if _, ok := pr.GetAnnotations()[a]; !ok {
			t.Errorf("annotation %s missing", a)
		}
	}

	t.Run("Result", func(t *testing.T) {
		name := pr.GetAnnotations()[annotation.Result]
		if _, err := results.GetResult(ctx, &pb.GetResultRequest{Name: name}); err != nil {
			t.Fatalf("GetResult: %v", err)
		}
	})

	t.Run("Record", func(t *testing.T) {
		name := pr.GetAnnotations()[annotation.Record]
		_, err := results.GetRecord(ctx, &pb.GetRecordRequest{Name: name})
		if err != nil {
			t.Fatalf("GetRecord: %v", err)
		}
	})

	// We don't do the same exhaustive feature testing as TaskRuns here -
	// since everything is handled as a generic object testing TaskRuns should
	// be sufficient coverage.
}
