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
	"testing"

	pipelinev1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/results/pkg/watcher/reconciler"
	resultsannotation "github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apis "knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
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
