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
	"testing"

	pipelinev1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	pipelinev1beta1listers "github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1beta1"
	resultsannotation "github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"knative.dev/pkg/logging"
)

func TestAreAllUnderlyingTaskRunsReadyForDeletion(t *testing.T) {
	tests := []struct {
		name string
		in   *pipelinev1beta1.PipelineRun
		want bool
	}{{
		name: "all underlying TaskRuns are ready to be deleted",
		in: &pipelinev1beta1.PipelineRun{
			Status: pipelinev1beta1.PipelineRunStatus{
				PipelineRunStatusFields: pipelinev1beta1.PipelineRunStatusFields{
					ChildReferences: []pipelinev1beta1.ChildStatusReference{{
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
			in: &pipelinev1beta1.PipelineRun{
				Status: pipelinev1beta1.PipelineRunStatus{
					PipelineRunStatusFields: pipelinev1beta1.PipelineRunStatusFields{
						ChildReferences: []pipelinev1beta1.ChildStatusReference{{
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
			in: &pipelinev1beta1.PipelineRun{
				Status: pipelinev1beta1.PipelineRunStatus{
					PipelineRunStatusFields: pipelinev1beta1.PipelineRunStatusFields{
						ChildReferences: []pipelinev1beta1.ChildStatusReference{{
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
		{
			name: "support full embedded status",
			in: &pipelinev1beta1.PipelineRun{
				Status: pipelinev1beta1.PipelineRunStatus{
					PipelineRunStatusFields: pipelinev1beta1.PipelineRunStatusFields{
						TaskRuns: map[string]*pipelinev1beta1.PipelineRunTaskRunStatus{
							"bar": &pipelinev1beta1.PipelineRunTaskRunStatus{},
						},
					},
				},
			},
			want: false,
		},
	}

	indexer := cache.NewIndexer(cache.DeletionHandlingMetaNamespaceKeyFunc, cache.Indexers{})

	// Put a few objects into the indexer.
	if err := indexer.Add(&pipelinev1beta1.TaskRun{
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

	if err := indexer.Add(&pipelinev1beta1.TaskRun{
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
				taskRunLister: pipelinev1beta1listers.NewTaskRunLister(indexer),
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
