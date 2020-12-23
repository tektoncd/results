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

package reconciler

import (
	"context"
	"testing"
	"time"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/pkg/client/clientset/versioned/fake"
	fakepipelineclient "github.com/tektoncd/pipeline/pkg/client/injection/client/fake"
	pipelineruninformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1beta1/pipelinerun"
	taskruninformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1beta1/taskrun"
	rtesting "github.com/tektoncd/pipeline/pkg/reconciler/testing"
	"github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	"github.com/tektoncd/results/pkg/watcher/reconciler/internal/test"
	"github.com/tektoncd/results/pkg/watcher/reconciler/pipelinerun"
	"github.com/tektoncd/results/pkg/watcher/reconciler/taskrun"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/controller"
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
	results := test.NewResultsClient(t)
	trctrl := taskrun.NewController(ctx, nil, results)
	prctrl := pipelinerun.NewController(ctx, nil, results)
	go controller.StartAll(ctx, trctrl, prctrl)

	// Start informers - this notifies the controller of new events.
	go taskruninformer.Get(ctx).Informer().Run(ctx.Done())
	go pipelineruninformer.Get(ctx).Informer().Run(ctx.Done())

	pipeline := fakepipelineclient.Get(ctx)
	t.Run("taskrun", func(t *testing.T) {
		reconcileTaskRun(ctx, t, pipeline)
	})
	t.Run("pipelinerun", func(t *testing.T) {
		reconcilePipelineRun(ctx, t, pipeline)
	})
}

func reconcileTaskRun(ctx context.Context, t *testing.T, client *fake.Clientset) {
	tr, err := client.TektonV1beta1().TaskRuns("ns").Create(&v1beta1.TaskRun{
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
	})
	if err != nil {
		t.Fatal(err)
	}

	// Wait for Result annotations to show up on the reconciled object.
	tick := time.NewTicker(1 * time.Second)
	select {
	case <-tick.C:
		tr, err = client.TektonV1beta1().TaskRuns("ns").Get(tr.GetName(), metav1.GetOptions{})
		if err == nil && tr.Annotations[annotation.ResultID] != "" {
			break
		}
	case <-ctx.Done():
		t.Fatalf("timed out. Last TaskRun: %+v", tr)
	}
}

func reconcilePipelineRun(ctx context.Context, t *testing.T, client *fake.Clientset) {
	pr, err := client.TektonV1beta1().PipelineRuns("ns").Create(&v1beta1.PipelineRun{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1beta1",
			Kind:       "pipelinerun",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "pr",
			Namespace:   "ns",
			Annotations: map[string]string{"demo": "demo"},
			UID:         "12345",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Wait for Result annotations to show up on the reconciled object.
	tick := time.NewTicker(1 * time.Second)
	select {
	case <-tick.C:
		pr, err = client.TektonV1beta1().PipelineRuns("ns").Get(pr.GetName(), metav1.GetOptions{})
		if err == nil && pr.Annotations[annotation.ResultID] != "" {
			break
		}
	case <-ctx.Done():
		t.Fatalf("timed out. Last PipelineRun: %+v", pr)
	}
}
