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

package taskrun

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/pkg/client/clientset/versioned/fake"
	fakepipelineclient "github.com/tektoncd/pipeline/pkg/client/injection/client/fake"
	rtesting "github.com/tektoncd/pipeline/pkg/reconciler/testing"
	"github.com/tektoncd/results/pkg/watcher/convert"
	"github.com/tektoncd/results/pkg/watcher/internal/test"
	"github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/protobuf/testing/protocmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
	"knative.dev/pkg/controller"

	// Needed for informer injection.
	_ "github.com/tektoncd/pipeline/test"
)

type env struct {
	ctx      context.Context
	ctrl     *controller.Impl
	results  pb.ResultsClient
	pipeline *fake.Clientset
}

func newEnv(t *testing.T) *env {
	t.Helper()

	// Configures fake tekton clients + informers.
	ctx, _ := rtesting.SetupFakeContext(t)

	results := test.NewResultsClient(t)
	ctrl := NewController(ctx, results)

	pipeline := fakepipelineclient.Get(ctx)

	return &env{
		ctx:      ctx,
		ctrl:     ctrl,
		results:  results,
		pipeline: pipeline,
	}
}

func TestReconcile(t *testing.T) {
	env := newEnv(t)

	tr, err := env.pipeline.TektonV1beta1().TaskRuns("ns").Create(&v1beta1.TaskRun{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1beta1",
			Kind:       "taskrun",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "Tekton-TaskRun",
			Namespace:   "ns",
			Annotations: map[string]string{"demo": "demo"},
			UID:         "12345",
		},
		Status: v1beta1.TaskRunStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{
					apis.Condition{
						Type:   apis.ConditionSucceeded,
						Status: corev1.ConditionFalse,
					},
				},
			},
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{},
		},
		Spec: v1beta1.TaskRunSpec{
			TaskSpec: &v1beta1.TaskSpec{
				Steps: []v1beta1.Step{{
					Script: "echo hello world!",
				}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("create", func(t *testing.T) {
		tr = reconcile(t, env, tr)
	})

	t.Run("nop", func(t *testing.T) {
		// This is treated like an update, even though there is no change.
		reconcile(t, env, tr)
	})

	t.Run("update", func(t *testing.T) {
		tr, err = env.pipeline.TektonV1beta1().TaskRuns(tr.GetNamespace()).Update(tr)
		if err != nil {
			t.Fatalf("TaskRun.Update: %v", err)
		}
		reconcile(t, env, tr)
	})
}

// reconcile forces a reconcile for the given TaskRun, and returns the newest
// TaskRun post-reconcile.
func reconcile(t *testing.T, env *env, want *v1beta1.TaskRun) *v1beta1.TaskRun {
	if err := env.ctrl.Reconciler.Reconcile(env.ctx, want.GetNamespacedName().String()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	// Verify that the TaskRun now has a Result annotation associated with it.
	tr, err := env.pipeline.TektonV1beta1().TaskRuns(want.GetNamespace()).Get(want.GetName(), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("TaskRun.Get(%s): %v", tr.Name, err)
	}
	for _, a := range []string{annotation.Result, annotation.Record} {
		if _, ok := tr.Annotations[a]; !ok {
			t.Errorf("annotation %s missing", a)
		}
	}

	// Verify Result data matches TaskRun.
	got, err := env.results.GetRecord(env.ctx, &pb.GetRecordRequest{Name: tr.Annotations[annotation.Record]})
	if err != nil {
		t.Fatalf("GetRecord: %v", err)
	}
	// We diff the base since we're storing the current state. We don't include
	// the result annotations since that's part of the "next" state.
	wantpb, err := convert.ToProto(want)
	if err != nil {
		t.Fatalf("convert.ToProto: %v", err)
	}
	if diff := cmp.Diff(wantpb, got.GetData(), protocmp.Transform()); diff != "" {
		t.Errorf("Result diff (-want, +got):\n%s", diff)
	}

	return tr
}
