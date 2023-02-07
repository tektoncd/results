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

package reconciler_test

import (
	"context"
	"github.com/tektoncd/results/pkg/api/server/config"
	"testing"
	"time"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	fakepipelineclient "github.com/tektoncd/pipeline/pkg/client/injection/client/fake"
	pipelineruninformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1beta1/pipelinerun"
	taskruninformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1beta1/taskrun"
	rtesting "github.com/tektoncd/pipeline/pkg/reconciler/testing"
	"github.com/tektoncd/results/pkg/internal/test"
	"github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	"github.com/tektoncd/results/pkg/watcher/reconciler/pipelinerun"
	"github.com/tektoncd/results/pkg/watcher/reconciler/taskrun"
	"github.com/tektoncd/results/pkg/watcher/results"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/controller"

	// Needed for informer injection.
	_ "github.com/tektoncd/pipeline/test"
)

// TestController starts a full TaskRun + PipelineRun controller and waits for
// objects to be reconciled. Unlike the individual controller tests that call
// Reconcile directly, this test is asynchronous and is slower as a result.
// If possible, prefer adding synchronous tests to the individual reconcilers.
func TestController(t *testing.T) {
	ctx, _ := rtesting.SetupFakeContext(t)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Create reconcilers, start controller.
	resultClient, _ := test.NewResultsClient(t, &config.Config{})

	trctrl := taskrun.NewController(ctx, resultClient)
	prctrl := pipelinerun.NewController(ctx, resultClient)
	go controller.StartAll(ctx, trctrl, prctrl)

	// Start informers - this notifies the controller of new events.
	go taskruninformer.Get(ctx).Informer().Run(ctx.Done())
	go pipelineruninformer.Get(ctx).Informer().Run(ctx.Done())

	pipeline := fakepipelineclient.Get(ctx)
	t.Run("taskrun", func(t *testing.T) {
		tr := &v1beta1.TaskRun{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "tekton.dev/v1beta1",
				Kind:       "TaskRun",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "taskrun",
				Namespace: "ns",
				Annotations: map[string]string{
					"demo": "demo",
					// This TaskRun belongs to a PipelineRun, so the record should
					// be associated with the PipelineRun result.
					"tekton.dev/pipelineRun": "pr",
				},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "tekton.dev/v1beta1",
					Kind:       "PipelineRun",
					UID:        "pr-id",
				}},
				UID: "tr-id",
			},
		}
		if _, err := pipeline.TektonV1beta1().TaskRuns(tr.GetNamespace()).Create(ctx, tr, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}

		get := func(context.Context) (results.Object, error) {
			return pipeline.TektonV1beta1().TaskRuns(tr.GetNamespace()).Get(ctx, tr.GetName(), metav1.GetOptions{})
		}
		wait(ctx, t, get, tr, "ns/results/pr-id")
	})

	t.Run("pipelinerun", func(t *testing.T) {
		pr := &v1beta1.PipelineRun{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "tekton.dev/v1beta1",
				Kind:       "PipelineRun",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        "pr",
				Namespace:   "ns",
				Annotations: map[string]string{"demo": "demo"},
				UID:         "pr-id",
			},
		}
		if _, err := pipeline.TektonV1beta1().PipelineRuns(pr.GetNamespace()).Create(ctx, pr, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}

		get := func(context.Context) (results.Object, error) {
			return pipeline.TektonV1beta1().PipelineRuns(pr.GetNamespace()).Get(ctx, pr.GetName(), metav1.GetOptions{})
		}
		wait(ctx, t, get, pr, "ns/results/pr-id")
	})
}

type getFn func(ctx context.Context) (results.Object, error)

func wait(ctx context.Context, t *testing.T, get getFn, o results.Object, want string) {
	// Wait for Result annotations to show up on the reconciled object.
	var u results.Object
	tick := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-tick.C:
			var err error
			u, err = get(ctx)
			t.Logf("Get (%v, %v)", u, err)
			if err != nil {
				t.Log(err)
				continue
			}

			if got := u.GetAnnotations()[annotation.Result]; err == nil && got != "" {
				if got != want {
					t.Fatalf("want result ID %s, got %s", want, got)
				}
				return
			}
		case <-ctx.Done():
			t.Fatalf("timed out. Last seen object: %+v", u)
		}
	}
}
