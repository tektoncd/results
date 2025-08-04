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

package annotation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// Mock client for testing
type mockObjectClient struct {
	patchCalled bool
	lastPatch   []byte
	lastOptions metav1.PatchOptions
	returnError error
}

func (m *mockObjectClient) Delete(_ context.Context, _ string, _ metav1.DeleteOptions) error {
	return nil
}

func (m *mockObjectClient) Patch(_ context.Context, _ string, _ types.PatchType, data []byte, opts metav1.PatchOptions, _ ...string) error {
	m.patchCalled = true
	m.lastPatch = data
	m.lastOptions = opts
	return m.returnError
}

func TestPatch(t *testing.T) {
	const (
		fakeResultID = "foo/results/bar"
		fakeRecordID = "foo/results/bar/records/baz"
	)

	annotations := []Annotation{
		{Name: Result, Value: fakeResultID},
		{Name: Record, Value: fakeRecordID},
	}

	tests := []struct {
		name        string
		object      metav1.Object
		annotations []Annotation
		clientError error
		wantError   bool
		wantPatched bool
		wantPatch   applyPatch
	}{
		{
			name: "successful patch for PipelineRun",
			object: func() metav1.Object {
				pr := &pipelinev1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pr",
						Namespace: "test-ns",
					},
				}
				pr.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "tekton.dev",
					Version: "v1",
					Kind:    "PipelineRun",
				})
				return pr
			}(),
			annotations: annotations,
			wantPatched: true,
			wantPatch: applyPatch{
				APIVersion: "tekton.dev/v1",
				Kind:       "PipelineRun",
				Metadata: metadata{
					Name:      "test-pr",
					Namespace: "test-ns",
					Annotations: map[string]string{
						Result: fakeResultID,
						Record: fakeRecordID,
					},
				},
			},
		},
		{
			name: "successful patch for TaskRun",
			object: func() metav1.Object {
				tr := &pipelinev1.TaskRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-tr",
						Namespace: "test-ns",
					},
				}
				tr.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "tekton.dev",
					Version: "v1",
					Kind:    "TaskRun",
				})
				return tr
			}(),
			annotations: annotations,
			wantPatched: true,
			wantPatch: applyPatch{
				APIVersion: "tekton.dev/v1",
				Kind:       "TaskRun",
				Metadata: metadata{
					Name:      "test-tr",
					Namespace: "test-ns",
					Annotations: map[string]string{
						Result: fakeResultID,
						Record: fakeRecordID,
					},
				},
			},
		},
		{
			name: "preserve existing managed annotations only",
			object: func() metav1.Object {
				pr := &pipelinev1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pr",
						Namespace: "test-ns",
						Annotations: map[string]string{
							"existing.annotation":       "existing-value", // Not managed by us
							"another.annotation":        "another-value",  // Not managed by us
							"results.tekton.dev/log":    "existing-log",   // Managed by us - should be preserved
							"results.tekton.dev/stored": "false",          // Managed by us - should be preserved
						},
					},
				}
				pr.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "tekton.dev",
					Version: "v1",
					Kind:    "PipelineRun",
				})
				return pr
			}(),
			annotations: annotations,
			wantPatched: true,
			wantPatch: applyPatch{
				APIVersion: "tekton.dev/v1",
				Kind:       "PipelineRun",
				Metadata: metadata{
					Name:      "test-pr",
					Namespace: "test-ns",
					Annotations: map[string]string{
						// Only our managed annotations should be included
						"results.tekton.dev/log":    "existing-log", // Preserved existing managed annotation
						"results.tekton.dev/stored": "false",        // Preserved existing managed annotation
						Result:                      fakeResultID,   // New annotation
						Record:                      fakeRecordID,   // New annotation
					},
				},
			},
		},
		{
			name: "skip patching when already patched",
			object: func() metav1.Object {
				pr := &pipelinev1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pr",
						Namespace: "test-ns",
						Annotations: map[string]string{
							Result: fakeResultID,
							Record: fakeRecordID,
						},
					},
				}
				pr.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "tekton.dev",
					Version: "v1",
					Kind:    "PipelineRun",
				})
				return pr
			}(),
			annotations: annotations,
			wantPatched: false, // Should skip patching
		},
		{
			name: "error when GVK is empty",
			object: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "test-ns",
				},
			},
			annotations: annotations,
			wantError:   true,
			wantPatched: false,
		},
		{
			name: "error from client",
			object: func() metav1.Object {
				pr := &pipelinev1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pr",
						Namespace: "test-ns",
					},
				}
				pr.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "tekton.dev",
					Version: "v1",
					Kind:    "PipelineRun",
				})
				return pr
			}(),
			annotations: annotations,
			clientError: errors.New("patch failed"),
			wantError:   true,
			wantPatched: true, // Patch should be attempted
		},
		{
			name: "skip empty annotation values",
			object: func() metav1.Object {
				pr := &pipelinev1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pr",
						Namespace: "test-ns",
					},
				}
				pr.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "tekton.dev",
					Version: "v1",
					Kind:    "PipelineRun",
				})
				return pr
			}(),
			annotations: []Annotation{
				{Name: Result, Value: fakeResultID},
				{Name: Record, Value: ""},  // Empty value should be skipped
				{Name: Log, Value: "test"}, // Non-empty should be included
			},
			wantPatched: true,
			wantPatch: applyPatch{
				APIVersion: "tekton.dev/v1",
				Kind:       "PipelineRun",
				Metadata: metadata{
					Name:      "test-pr",
					Namespace: "test-ns",
					Annotations: map[string]string{
						Result: fakeResultID,
						Log:    "test",
						// Record should not be present due to empty value
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockObjectClient{returnError: tt.clientError}
			ctx := context.Background()

			err := Patch(ctx, tt.object, client, tt.annotations...)

			// Check error expectations
			if tt.wantError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Check if patch was called
			if tt.wantPatched != client.patchCalled {
				t.Errorf("expected patch called: %v, got: %v", tt.wantPatched, client.patchCalled)
			}

			// If patch was expected and called, verify the patch content
			if tt.wantPatched && client.patchCalled && err == nil {
				var actualPatch applyPatch
				if err := json.Unmarshal(client.lastPatch, &actualPatch); err != nil {
					t.Fatalf("failed to unmarshal patch: %v", err)
				}

				if diff := cmp.Diff(tt.wantPatch, actualPatch); diff != "" {
					t.Errorf("patch mismatch (-want +got):\n%s", diff)
				}

				// Verify patch options
				if client.lastOptions.FieldManager != fieldManager {
					t.Errorf("expected field manager %q, got %q", fieldManager, client.lastOptions.FieldManager)
				}
				if client.lastOptions.Force == nil || !*client.lastOptions.Force {
					t.Errorf("expected Force=true, got %v", client.lastOptions.Force)
				}

				// Verify in-memory object was updated
				objAnnotations := tt.object.GetAnnotations()
				for _, ann := range tt.annotations {
					if ann.Value != "" && objAnnotations[ann.Name] != ann.Value {
						t.Errorf("annotation %s not updated in object, expected %q, got %q",
							ann.Name, ann.Value, objAnnotations[ann.Name])
					}
				}
			}
		})
	}
}

func TestIsPatched(t *testing.T) {
	const (
		fakeResultID = "foo/results/bar"
		fakeRecordID = "foo/results/bar/records/baz"
	)

	annotations := []Annotation{
		{Name: Result, Value: fakeResultID},
		{Name: Record, Value: fakeRecordID},
	}

	tests := []struct {
		name        string
		object      metav1.Object
		annotations []Annotation
		want        bool
	}{
		{
			name: "no annotations present",
			object: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{},
			},
			annotations: annotations,
			want:        false,
		},
		{
			name: "partial annotations present",
			object: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						Result: fakeResultID,
						// Record is missing
					},
				},
			},
			annotations: annotations,
			want:        false,
		},
		{
			name: "all annotations present",
			object: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						Result: fakeResultID,
						Record: fakeRecordID,
					},
				},
			},
			annotations: annotations,
			want:        true,
		},
		{
			name: "all annotations present with extras",
			object: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						Result:               fakeResultID,
						Record:               fakeRecordID,
						"extra.annotation":   "extra-value",
						"another.annotation": "another-value",
					},
				},
			},
			annotations: annotations,
			want:        true,
		},
		{
			name: "annotation value mismatch",
			object: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						Result: "wrong-value",
						Record: fakeRecordID,
					},
				},
			},
			annotations: annotations,
			want:        false,
		},
		{
			name: "nil annotations map",
			object: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: nil,
				},
			},
			annotations: annotations,
			want:        false,
		},
		{
			name: "empty annotations list",
			object: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						Result: fakeResultID,
						Record: fakeRecordID,
					},
				},
			},
			annotations: []Annotation{}, // Empty list
			want:        true,           // Should return true for empty list
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPatched(tt.object, tt.annotations...)
			if got != tt.want {
				t.Errorf("IsPatched() = %v, want %v", got, tt.want)
			}
		})
	}
}
