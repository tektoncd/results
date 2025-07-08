// Copyright 2023 The Tekton Authors
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
	"errors"
	"fmt"
	"testing"
	"time"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	pipelinev1listers "github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1"
	"github.com/tektoncd/results/pkg/watcher/reconciler"
	resultsannotation "github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	apis "knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	knativereconciler "knative.dev/pkg/reconciler"
)

func TestReconcile(t *testing.T) {
	for _, tc := range []struct {
		name string
		pr   *pipelinev1.PipelineRun
		cfg  *reconciler.Config
		want knativereconciler.Event
	}{
		{
			name: "incomplete run with disable storing - skip",
			pr: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "test-ns",
				},
				Status: pipelinev1.PipelineRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							apis.Condition{
								Type:   apis.ConditionSucceeded,
								Status: corev1.ConditionUnknown,
							},
						},
					},
				},
			},
			cfg: &reconciler.Config{
				DisableStoringIncompleteRuns: true,
			},
			want: nil,
		},
		{
			name: "already stored - skip",
			pr: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "test-ns",
					Annotations: map[string]string{
						resultsannotation.Stored: "true",
					},
				},
				Status: pipelinev1.PipelineRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							apis.Condition{
								Type:   apis.ConditionSucceeded,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			cfg: &reconciler.Config{
				DisableStoringIncompleteRuns: true,
			},
			want: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			ctx = logging.WithLogger(ctx, zaptest.NewLogger(t).Sugar())

			r := &Reconciler{
				cfg: tc.cfg,
			}
			got := r.ReconcileKind(ctx, tc.pr)
			if got != tc.want {
				t.Errorf("ReconcileKind() = %v, want %v", got, tc.want)
			}
		})
	}
}
func TestAreAllUnderlyingTaskRunsReadyForDeletion(t *testing.T) {
	tests := []struct {
		name string
		in   *pipelinev1.PipelineRun
		want bool
	}{{
		name: "all underlying TaskRuns are ready to be deleted",
		in: &pipelinev1.PipelineRun{
			Status: pipelinev1.PipelineRunStatus{
				PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
					ChildReferences: []pipelinev1.ChildStatusReference{{
						Name: "foo",
					},
					},
				},
			},
		},
		want: true,
	},
		{
			name: "one TaskRun is ready to be deleted whereas the other is not",
			in: &pipelinev1.PipelineRun{
				Status: pipelinev1.PipelineRunStatus{
					PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
						ChildReferences: []pipelinev1.ChildStatusReference{{
							Name: "foo",
						},
							{
								Name: "bar",
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "consider that missing TaskRuns can be deleted",
			in: &pipelinev1.PipelineRun{
				Status: pipelinev1.PipelineRunStatus{
					PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
						ChildReferences: []pipelinev1.ChildStatusReference{{
							Name: "foo",
						},
							{
								Name: "baz",
							},
						},
					},
				},
			},
			want: true,
		},
	}

	indexer := cache.NewIndexer(cache.DeletionHandlingMetaNamespaceKeyFunc, cache.Indexers{})

	// Put a few objects into the indexer.
	if err := indexer.Add(&pipelinev1.TaskRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: corev1.NamespaceDefault,
			Annotations: map[string]string{
				resultsannotation.ChildReadyForDeletion: "true",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if err := indexer.Add(&pipelinev1.TaskRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: corev1.NamespaceDefault,
		},
	}); err != nil {
		t.Fatal(err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reconciler := &Reconciler{
				taskRunLister: pipelinev1listers.NewTaskRunLister(indexer),
			}

			test.in.Namespace = corev1.NamespaceDefault

			ctx := context.Background()
			ctx = logging.WithLogger(ctx, zaptest.NewLogger(t).Sugar())
			got, err := reconciler.areAllUnderlyingTaskRunsReadyForDeletion(ctx, test.in)
			if err != nil {
				t.Fatal(err)
			}

			if test.want != got {
				t.Fatalf("Want %t, but got %t", test.want, got)
			}
		})
	}
}

func TestFinalize(t *testing.T) {
	storeDeadline := time.Hour
	finalizerRequeueInterval := time.Second

	cfg := &reconciler.Config{
		StoreDeadline:            &storeDeadline,
		FinalizerRequeueInterval: finalizerRequeueInterval,
	}

	for _, tc := range []struct {
		name           string
		pr             *pipelinev1.PipelineRun
		cfg            *reconciler.Config
		reconcileError knativereconciler.Event
		want           knativereconciler.Event
	}{
		{
			name: "pipelinerun still running - skip finalization",
			pr: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "test-ns",
				},
				Status: pipelinev1.PipelineRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							apis.Condition{
								Type:   apis.ConditionSucceeded,
								Status: corev1.ConditionUnknown,
							},
						},
					},
				},
			},
			cfg:  cfg,
			want: nil,
		},
		{
			name: "store deadline passed - proceed with deletion",
			pr: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "test-ns",
				},
				Status: pipelinev1.PipelineRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							apis.Condition{
								Type:   apis.ConditionSucceeded,
								Status: corev1.ConditionTrue,
							},
						},
					},
					PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
						CompletionTime: &metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
					},
				},
			},
			cfg:  cfg,
			want: nil,
		},
		{
			name: "missing annotations - requeue",
			pr: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "test-ns",
				},
				Status: pipelinev1.PipelineRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							apis.Condition{
								Type:   apis.ConditionSucceeded,
								Status: corev1.ConditionTrue,
							},
						},
					},
					PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
						CompletionTime: &metav1.Time{Time: time.Now()},
					},
				},
			},
			cfg:  cfg,
			want: controller.NewRequeueAfter(finalizerRequeueInterval),
		},
		{
			name: "stored annotation missing - requeue",
			pr: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "test-ns",
					Annotations: map[string]string{
						"demo": "demo",
					},
				},
				Status: pipelinev1.PipelineRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							apis.Condition{
								Type:   apis.ConditionSucceeded,
								Status: corev1.ConditionTrue,
							},
						},
					},
					PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
						CompletionTime: &metav1.Time{Time: time.Now()},
					},
				},
			},
			cfg:  cfg,
			want: controller.NewRequeueAfter(finalizerRequeueInterval),
		},
		{
			name: "stored annotation not true - requeue",
			pr: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "test-ns",
					Annotations: map[string]string{
						resultsannotation.Stored: "false",
					},
				},
				Status: pipelinev1.PipelineRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							apis.Condition{
								Type:   apis.ConditionSucceeded,
								Status: corev1.ConditionTrue,
							},
						},
					},
					PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
						CompletionTime: &metav1.Time{Time: time.Now()},
					},
				},
			},
			cfg:  cfg,
			want: controller.NewRequeueAfter(finalizerRequeueInterval),
		},
		{
			name: "reconcile error - requeue",
			pr: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "test-ns",
					Annotations: map[string]string{
						resultsannotation.Stored: "true",
					},
				},
				Status: pipelinev1.PipelineRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							apis.Condition{
								Type:   apis.ConditionSucceeded,
								Status: corev1.ConditionTrue,
							},
						},
					},
					PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
						CompletionTime: &metav1.Time{Time: time.Now()},
					},
				},
			},
			cfg:            cfg,
			reconcileError: fmt.Errorf("reconcile error"),
			want:           controller.NewRequeueAfter(finalizerRequeueInterval),
		},
		{
			name: "successful finalization",
			pr: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "test-ns",
					Annotations: map[string]string{
						resultsannotation.Stored: "true",
					},
				},
				Status: pipelinev1.PipelineRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							apis.Condition{
								Type:   apis.ConditionSucceeded,
								Status: corev1.ConditionTrue,
							},
						},
					},
					PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
						CompletionTime: &metav1.Time{Time: time.Now()},
					},
				},
			},
			cfg:  cfg,
			want: nil,
		},
		{
			name: "verify finalizer requeue interval",
			pr: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "test-ns",
				},
				Status: pipelinev1.PipelineRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							apis.Condition{
								Type:   apis.ConditionSucceeded,
								Status: corev1.ConditionTrue,
							},
						},
					},
					PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
						CompletionTime: &metav1.Time{Time: time.Now()},
					},
				},
			},
			cfg: &reconciler.Config{
				FinalizerRequeueInterval: finalizerRequeueInterval,
			},
			want: controller.NewRequeueAfter(finalizerRequeueInterval),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			ctx = logging.WithLogger(ctx, zaptest.NewLogger(t).Sugar())

			r := &Reconciler{
				cfg: tc.cfg,
			}

			got := r.finalize(ctx, tc.pr, tc.reconcileError)
			if !errors.Is(got, tc.want) {
				t.Errorf("finalize() = %v, want %v", got, tc.want)
			}
		})
	}
}
