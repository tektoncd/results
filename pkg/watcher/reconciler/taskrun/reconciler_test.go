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

package taskrun

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tektoncd/results/pkg/watcher/reconciler"
	resultsannotation "github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apis "knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	knativereconciler "knative.dev/pkg/reconciler"
)

func TestFinalize(t *testing.T) {
	storeDeadline := time.Hour
	finalizerRequeueInterval := time.Second

	cfg := &reconciler.Config{
		StoreDeadline:            &storeDeadline,
		FinalizerRequeueInterval: finalizerRequeueInterval,
	}

	for _, tc := range []struct {
		name           string
		pr             *pipelinev1.TaskRun
		cfg            *reconciler.Config
		reconcileError knativereconciler.Event
		want           knativereconciler.Event
	}{
		{
			name: "taskrun still running - skip finalization",
			pr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "test-ns",
				},
				Status: pipelinev1.TaskRunStatus{
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
			pr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "test-ns",
				},
				Status: pipelinev1.TaskRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							apis.Condition{
								Type:   apis.ConditionSucceeded,
								Status: corev1.ConditionTrue,
							},
						},
					},
					TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
						CompletionTime: &metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
					},
				},
			},
			cfg:  cfg,
			want: nil,
		},
		{
			name: "missing annotations - requeue",
			pr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "test-ns",
				},
				Status: pipelinev1.TaskRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							apis.Condition{
								Type:   apis.ConditionSucceeded,
								Status: corev1.ConditionTrue,
							},
						},
					},
					TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
						CompletionTime: &metav1.Time{Time: time.Now()},
					},
				},
			},
			cfg:  cfg,
			want: controller.NewRequeueAfter(finalizerRequeueInterval),
		},
		{
			name: "stored annotation missing - requeue",
			pr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "test-ns",
					Annotations: map[string]string{
						"demo": "demo",
					},
				},
				Status: pipelinev1.TaskRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							apis.Condition{
								Type:   apis.ConditionSucceeded,
								Status: corev1.ConditionTrue,
							},
						},
					},
					TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
						CompletionTime: &metav1.Time{Time: time.Now()},
					},
				},
			},
			cfg:  cfg,
			want: controller.NewRequeueAfter(finalizerRequeueInterval),
		},
		{
			name: "stored annotation not true - requeue",
			pr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "test-ns",
					Annotations: map[string]string{
						resultsannotation.Stored: "false",
					},
				},
				Status: pipelinev1.TaskRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							apis.Condition{
								Type:   apis.ConditionSucceeded,
								Status: corev1.ConditionTrue,
							},
						},
					},
					TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
						CompletionTime: &metav1.Time{Time: time.Now()},
					},
				},
			},
			cfg:  cfg,
			want: controller.NewRequeueAfter(finalizerRequeueInterval),
		},
		{
			name: "reconcile error - requeue",
			pr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "test-ns",
					Annotations: map[string]string{
						resultsannotation.Stored: "true",
					},
				},
				Status: pipelinev1.TaskRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							apis.Condition{
								Type:   apis.ConditionSucceeded,
								Status: corev1.ConditionTrue,
							},
						},
					},
					TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
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
			pr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "test-ns",
					Annotations: map[string]string{
						resultsannotation.Stored: "true",
					},
				},
				Status: pipelinev1.TaskRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							apis.Condition{
								Type:   apis.ConditionSucceeded,
								Status: corev1.ConditionTrue,
							},
						},
					},
					TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
						CompletionTime: &metav1.Time{Time: time.Now()},
					},
				},
			},
			cfg:  cfg,
			want: nil,
		},
		{
			name: "verify finalizer requeue interval",
			pr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "test-ns",
				},
				Status: pipelinev1.TaskRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							apis.Condition{
								Type:   apis.ConditionSucceeded,
								Status: corev1.ConditionTrue,
							},
						},
					},
					TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
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
