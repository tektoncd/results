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

package pipelinerun

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	pipelinetest "github.com/tektoncd/pipeline/test"
	"github.com/tektoncd/results/pkg/watcher/convert"
	"github.com/tektoncd/results/pkg/watcher/internal/test"
	"github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	pb "github.com/tektoncd/results/proto/v1alpha1/results_go_proto"
	"google.golang.org/protobuf/testing/protocmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type pipelineRunTest struct {
	pipelineRun *v1beta1.PipelineRun
	asset       pipelinetest.Assets
	ctx         context.Context
	client      pb.ResultsClient
}

func newPipelineRunTest(t *testing.T) *pipelineRunTest {
	client := test.NewLegacyResultsClient(t)
	pipelineRun := &v1beta1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "Tekton-PipelineRun",
			Namespace:   "default",
			Annotations: map[string]string{"demo": "pipelinerun_demo"},
			UID:         "54321",
		},
	}
	d := pipelinetest.Data{
		PipelineRuns: []*v1beta1.PipelineRun{pipelineRun},
	}
	ctx, tclients, cmw := test.GetFakeClients(t, d, client)
	pipelineRunTest := &pipelineRunTest{
		pipelineRun: pipelineRun,
		asset: pipelinetest.Assets{
			Controller: NewController(ctx, cmw, client),
			Clients:    tclients,
		},
		ctx:    ctx,
		client: client,
	}
	return pipelineRunTest
}

func TestReconcile_CreatePipelineRun(t *testing.T) {
	tt := newPipelineRunTest(t)

	pr, err := test.ReconcilePipelineRun(tt.ctx, tt.asset, tt.pipelineRun)
	if err != nil {
		t.Fatalf("Failed to get completed PipelineRun %s: %v", tt.pipelineRun.Name, err)
	}
	if _, ok := pr.Annotations[annotation.Result]; !ok {
		t.Fatalf("Expected completed PipelineRun %s should be updated with a results_id field in annotations", tt.pipelineRun.Name)
	}
	if _, err := tt.client.GetResult(tt.ctx, &pb.GetResultRequest{Name: pr.Annotations[annotation.Result]}); err != nil {
		t.Fatalf("Expected completed PipelineRun %s not created in api server", tt.pipelineRun.Name)
	}
}

func TestReconcile_UnchangePipelineRun(t *testing.T) {
	tt := newPipelineRunTest(t)

	// Reconcile once to get IDs, etc.
	pr, err := test.ReconcilePipelineRun(tt.ctx, tt.asset, tt.pipelineRun)
	if err != nil {
		t.Fatalf("failed to get PipelineRun %s: %v", tt.pipelineRun.Name, err)
	}

	// Reconcile again to verify nothing changes.
	newpr, err := test.ReconcilePipelineRun(tt.ctx, tt.asset, tt.pipelineRun)
	if err != nil {
		t.Fatalf("failed to get second PipelineRun %s: %v", tt.pipelineRun.Name, err)
	}
	if diff := cmp.Diff(pr, newpr); diff != "" {
		t.Fatal(diff)
	}
}

func TestReconcile_UpdatePipelineRun(t *testing.T) {
	tt := newPipelineRunTest(t)

	pr, err := test.ReconcilePipelineRun(tt.ctx, tt.asset, tt.pipelineRun)
	if err != nil {
		t.Fatalf("Failed to get completed PipelineRun %s: %v", tt.pipelineRun.Name, err)
	}
	pr.UID = "234435"
	if _, err := tt.asset.Clients.Pipeline.TektonV1beta1().PipelineRuns(tt.pipelineRun.Namespace).Update(pr); err != nil {
		t.Fatalf("Failed to update PipelineRun %s: %v", tt.pipelineRun.Name, err)
	}
	updatepr, err := test.ReconcilePipelineRun(tt.ctx, tt.asset, pr)
	if err != nil {
		t.Fatalf("Failed to reconcile PipelineRun %s: %v", tt.pipelineRun.Name, err)
	}
	updatepr.ResourceVersion = pr.ResourceVersion
	if diff := cmp.Diff(pr, updatepr); diff != "" {
		t.Fatalf("Expected completed PipelineRun should be updated in cluster: %v", diff)
	}
	res, err := tt.client.GetResult(tt.ctx, &pb.GetResultRequest{Name: pr.Annotations[annotation.Result]})
	if err != nil {
		t.Fatalf("Expected completed PipelineRun %s not created in api server", tt.pipelineRun.Name)
	}
	p, err := convert.ToPipelineRunProto(updatepr)
	if err != nil {
		t.Fatalf("failed to convert to proto: %v", err)
	}
	want := &pb.Result{
		Name: pr.Annotations[annotation.Result],
		Executions: []*pb.Execution{{
			Execution: &pb.Execution_PipelineRun{PipelineRun: p},
		}},
	}
	if diff := cmp.Diff(want, res, protocmp.Transform()); diff != "" {
		t.Fatalf("Expected completed PipelineRun should be upated in api server: %v", diff)
	}
}
