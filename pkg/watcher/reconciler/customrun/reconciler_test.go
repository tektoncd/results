// Copyright 2026 The Tekton Authors
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

package customrun

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	pipelinev1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	fakeversioned "github.com/tektoncd/pipeline/pkg/client/clientset/versioned/fake"
	"github.com/tektoncd/results/pkg/watcher/reconciler"
	resultsannotation "github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
	apis "knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	knativereconciler "knative.dev/pkg/reconciler"
)

func TestReconcile(t *testing.T) {
	for _, tc := range []struct {
		name string
		cr   *pipelinev1beta1.CustomRun
		cfg  *reconciler.Config
		want knativereconciler.Event
	}{
		{
			name: "incomplete run with disable storing - skip",
			cr: &pipelinev1beta1.CustomRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cr",
					Namespace: "test-ns",
				},
				Status: pipelinev1beta1.CustomRunStatus{
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
			cr: &pipelinev1beta1.CustomRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cr",
					Namespace: "test-ns",
					Annotations: map[string]string{
						resultsannotation.Stored: "true",
					},
				},
				Status: pipelinev1beta1.CustomRunStatus{
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
			got := r.ReconcileKind(ctx, tc.cr)
			if got != tc.want {
				t.Errorf("ReconcileKind() = %v, want %v", got, tc.want)
			}
		})
	}
}

func mergePatchFinalizerManagedFields(finalizerName string) []metav1.ManagedFieldsEntry {
	raw := fmt.Sprintf(`{"f:metadata":{"f:finalizers":{".":{},"v:\"%s\"":{}}}}`, finalizerName)
	return []metav1.ManagedFieldsEntry{{
		Operation: metav1.ManagedFieldsOperationUpdate,
		FieldsV1:  &metav1.FieldsV1{Raw: []byte(raw)},
	}}
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
		cr             *pipelinev1beta1.CustomRun
		cfg            *reconciler.Config
		reconcileError knativereconciler.Event
		patchErr       error
		want           knativereconciler.Event
	}{
		{
			name: "migration: merge-patch finalizer removed successfully",
			cr: &pipelinev1beta1.CustomRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-cr",
					Namespace:  "test-ns",
					Finalizers: []string{"results.tekton.dev/customrun"},
					Annotations: map[string]string{
						resultsannotation.Stored: "true",
					},
					ManagedFields: mergePatchFinalizerManagedFields("results.tekton.dev/customrun"),
				},
				Status: pipelinev1beta1.CustomRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							apis.Condition{
								Type:   apis.ConditionSucceeded,
								Status: corev1.ConditionTrue,
							},
						},
					},
					CustomRunStatusFields: pipelinev1beta1.CustomRunStatusFields{
						CompletionTime: &metav1.Time{Time: time.Now()},
					},
				},
			},
			cfg:  cfg,
			want: nil,
		},
		{
			name: "migration: merge-patch removal fails - requeue",
			cr: &pipelinev1beta1.CustomRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-cr",
					Namespace:  "test-ns",
					Finalizers: []string{"results.tekton.dev/customrun"},
					Annotations: map[string]string{
						resultsannotation.Stored: "true",
					},
					ManagedFields: mergePatchFinalizerManagedFields("results.tekton.dev/customrun"),
				},
				Status: pipelinev1beta1.CustomRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							apis.Condition{
								Type:   apis.ConditionSucceeded,
								Status: corev1.ConditionTrue,
							},
						},
					},
					CustomRunStatusFields: pipelinev1beta1.CustomRunStatusFields{
						CompletionTime: &metav1.Time{Time: time.Now()},
					},
				},
			},
			cfg:      cfg,
			patchErr: fmt.Errorf("connection refused"),
			want:     controller.NewRequeueAfter(finalizerRequeueInterval),
		},
		{
			name: "no migration: finalizer owned by SSA - no patch needed",
			cr: &pipelinev1beta1.CustomRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-cr",
					Namespace:  "test-ns",
					Finalizers: []string{"results.tekton.dev/customrun"},
					Annotations: map[string]string{
						resultsannotation.Stored: "true",
					},
					ManagedFields: []metav1.ManagedFieldsEntry{{
						Operation: metav1.ManagedFieldsOperationApply,
						FieldsV1:  &metav1.FieldsV1{Raw: []byte(`{"f:metadata":{"f:finalizers":{".":{},"v:\"results.tekton.dev/customrun\"":{}}}}`)},
					}},
				},
				Status: pipelinev1beta1.CustomRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							apis.Condition{
								Type:   apis.ConditionSucceeded,
								Status: corev1.ConditionTrue,
							},
						},
					},
					CustomRunStatusFields: pipelinev1beta1.CustomRunStatusFields{
						CompletionTime: &metav1.Time{Time: time.Now()},
					},
				},
			},
			cfg:  cfg,
			want: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			ctx = logging.WithLogger(ctx, zaptest.NewLogger(t).Sugar())

			r := &Reconciler{
				cfg: tc.cfg,
			}

			if tc.cr.ManagedFields != nil {
				fakeClient := fakeversioned.NewSimpleClientset(tc.cr)
				if tc.patchErr != nil {
					fakeClient.PrependReactor("patch", "customruns", func(_ k8stesting.Action) (bool, runtime.Object, error) {
						return true, nil, tc.patchErr
					})
				}
				r.pipelineClient = fakeClient
			}

			got := r.finalize(ctx, tc.cr, tc.reconcileError)
			if !errors.Is(got, tc.want) {
				t.Errorf("finalize() = %v, want %v", got, tc.want)
			}
		})
	}
}
