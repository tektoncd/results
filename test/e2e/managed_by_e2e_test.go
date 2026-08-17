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

//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"
)

// TestManagedByCustomControllerNotProcessed verifies that TaskRuns and
// PipelineRuns with a custom spec.managedBy value are ignored by the
// Results watcher. The watcher should not add its finalizer or
// results.tekton.dev annotations to these runs.
//
// Instead of sleeping, we create a "control" resource with the default
// managedBy and poll until the watcher processes it. Once the control
// resource has Results annotations, we know the watcher has had ample
// time to process anything in its queue — so the custom-managed resource
// should still be untouched.
func TestManagedByCustomControllerNotProcessed(t *testing.T) {
	ctx := context.Background()
	tc := tektonClient(t)

	t.Run("taskrun", func(t *testing.T) {
		// Create the custom-managed TaskRun (should be ignored).
		customTR := &tektonv1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "managed-by-custom-tr-",
			},
			Spec: tektonv1.TaskRunSpec{
				TaskSpec: &tektonv1.TaskSpec{
					Steps: []tektonv1.Step{{
						Name:    "hello",
						Image:   "alpine",
						Command: []string{"echo", "hello"},
					}},
				},
				ManagedBy: ptr.To("custom-controller"),
			},
		}
		createdCustom, err := tc.TaskRuns(defaultNamespace).Create(ctx, customTR, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create custom-managed TaskRun: %v", err)
		}
		t.Cleanup(func() {
			_ = tc.TaskRuns(defaultNamespace).Delete(ctx, createdCustom.Name, metav1.DeleteOptions{})
		})

		// Create a control TaskRun (default managedBy — should be processed).
		controlTR := &tektonv1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "managed-by-control-tr-",
			},
			Spec: tektonv1.TaskRunSpec{
				TaskSpec: &tektonv1.TaskSpec{
					Steps: []tektonv1.Step{{
						Name:    "hello",
						Image:   "alpine",
						Command: []string{"echo", "hello"},
					}},
				},
			},
		}
		createdControl, err := tc.TaskRuns(defaultNamespace).Create(ctx, controlTR, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create control TaskRun: %v", err)
		}
		t.Cleanup(func() {
			_ = tc.TaskRuns(defaultNamespace).Delete(ctx, createdControl.Name, metav1.DeleteOptions{})
		})

		// Wait for the control TaskRun to be processed by the watcher.
		if err := wait.PollUntilContextTimeout(ctx, 1*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
			got, err := tc.TaskRuns(defaultNamespace).Get(ctx, createdControl.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			_, hasResult := got.Annotations["results.tekton.dev/result"]
			_, hasRecord := got.Annotations["results.tekton.dev/record"]
			return hasResult && hasRecord, nil
		}); err != nil {
			t.Fatalf("Timed out waiting for Results annotations on control TaskRun: %v", err)
		}

		// Now assert the custom-managed TaskRun was NOT processed.
		got, err := tc.TaskRuns(defaultNamespace).Get(ctx, createdCustom.Name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Failed to get custom-managed TaskRun: %v", err)
		}
		for _, f := range got.Finalizers {
			if f == "results.tekton.dev/taskrun" {
				t.Errorf("Results finalizer %q should not be added to externally-managed TaskRun", f)
			}
		}
		for key := range got.Annotations {
			if key == "results.tekton.dev/result" || key == "results.tekton.dev/record" {
				t.Errorf("Annotation %q should not be set on externally-managed TaskRun", key)
			}
		}
	})

	t.Run("pipelinerun", func(t *testing.T) {
		// Create the custom-managed PipelineRun (should be ignored).
		customPR := &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "managed-by-custom-pr-",
			},
			Spec: tektonv1.PipelineRunSpec{
				PipelineSpec: &tektonv1.PipelineSpec{
					Tasks: []tektonv1.PipelineTask{{
						Name: "hello",
						TaskSpec: &tektonv1.EmbeddedTask{
							TaskSpec: tektonv1.TaskSpec{
								Steps: []tektonv1.Step{{
									Name:    "hello",
									Image:   "alpine",
									Command: []string{"echo", "hello"},
								}},
							},
						},
					}},
				},
				ManagedBy: ptr.To("custom-controller"),
			},
		}
		createdCustom, err := tc.PipelineRuns(defaultNamespace).Create(ctx, customPR, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create custom-managed PipelineRun: %v", err)
		}
		t.Cleanup(func() {
			_ = tc.PipelineRuns(defaultNamespace).Delete(ctx, createdCustom.Name, metav1.DeleteOptions{})
		})

		// Create a control PipelineRun (default managedBy — should be processed).
		controlPR := &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "managed-by-control-pr-",
			},
			Spec: tektonv1.PipelineRunSpec{
				PipelineSpec: &tektonv1.PipelineSpec{
					Tasks: []tektonv1.PipelineTask{{
						Name: "hello",
						TaskSpec: &tektonv1.EmbeddedTask{
							TaskSpec: tektonv1.TaskSpec{
								Steps: []tektonv1.Step{{
									Name:    "hello",
									Image:   "alpine",
									Command: []string{"echo", "hello"},
								}},
							},
						},
					}},
				},
			},
		}
		createdControl, err := tc.PipelineRuns(defaultNamespace).Create(ctx, controlPR, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create control PipelineRun: %v", err)
		}
		t.Cleanup(func() {
			_ = tc.PipelineRuns(defaultNamespace).Delete(ctx, createdControl.Name, metav1.DeleteOptions{})
		})

		// Wait for the control PipelineRun to be processed by the watcher.
		if err := wait.PollUntilContextTimeout(ctx, 1*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
			got, err := tc.PipelineRuns(defaultNamespace).Get(ctx, createdControl.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			_, hasResult := got.Annotations["results.tekton.dev/result"]
			_, hasRecord := got.Annotations["results.tekton.dev/record"]
			return hasResult && hasRecord, nil
		}); err != nil {
			t.Fatalf("Timed out waiting for Results annotations on control PipelineRun: %v", err)
		}

		// Now assert the custom-managed PipelineRun was NOT processed.
		got, err := tc.PipelineRuns(defaultNamespace).Get(ctx, createdCustom.Name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Failed to get custom-managed PipelineRun: %v", err)
		}
		for _, f := range got.Finalizers {
			if f == "results.tekton.dev/pipelinerun" {
				t.Errorf("Results finalizer %q should not be added to externally-managed PipelineRun", f)
			}
		}
		for key := range got.Annotations {
			if key == "results.tekton.dev/result" || key == "results.tekton.dev/record" {
				t.Errorf("Annotation %q should not be set on externally-managed PipelineRun", key)
			}
		}
	})
}

// TestManagedByDefaultProcessed verifies that TaskRuns and PipelineRuns
// with nil or default managedBy values are processed normally by the
// Results watcher.
func TestManagedByDefaultProcessed(t *testing.T) {
	ctx := context.Background()
	tc := tektonClient(t)

	tests := []struct {
		name      string
		managedBy *string
	}{
		{
			name:      "nil managedBy",
			managedBy: nil,
		},
		{
			name:      "tekton.dev/pipeline managedBy",
			managedBy: ptr.To("tekton.dev/pipeline"),
		},
	}

	for _, tt := range tests {
		t.Run("taskrun/"+tt.name, func(t *testing.T) {
			tr := &tektonv1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "managed-by-default-tr-",
				},
				Spec: tektonv1.TaskRunSpec{
					TaskSpec: &tektonv1.TaskSpec{
						Steps: []tektonv1.Step{{
							Name:    "hello",
							Image:   "alpine",
							Command: []string{"echo", "hello"},
						}},
					},
					ManagedBy: tt.managedBy,
				},
			}

			created, err := tc.TaskRuns(defaultNamespace).Create(ctx, tr, metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("Failed to create TaskRun: %v", err)
			}
			t.Cleanup(func() {
				_ = tc.TaskRuns(defaultNamespace).Delete(ctx, created.Name, metav1.DeleteOptions{})
			})

			if err := wait.PollUntilContextTimeout(ctx, 1*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
				got, err := tc.TaskRuns(defaultNamespace).Get(ctx, created.Name, metav1.GetOptions{})
				if err != nil {
					return false, err
				}
				_, hasResult := got.Annotations["results.tekton.dev/result"]
				_, hasRecord := got.Annotations["results.tekton.dev/record"]
				return hasResult && hasRecord, nil
			}); err != nil {
				t.Fatalf("Timed out waiting for Results annotations on TaskRun: %v", err)
			}
		})

		t.Run("pipelinerun/"+tt.name, func(t *testing.T) {
			pr := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "managed-by-default-pr-",
				},
				Spec: tektonv1.PipelineRunSpec{
					PipelineSpec: &tektonv1.PipelineSpec{
						Tasks: []tektonv1.PipelineTask{{
							Name: "hello",
							TaskSpec: &tektonv1.EmbeddedTask{
								TaskSpec: tektonv1.TaskSpec{
									Steps: []tektonv1.Step{{
										Name:    "hello",
										Image:   "alpine",
										Command: []string{"echo", "hello"},
									}},
								},
							},
						}},
					},
					ManagedBy: tt.managedBy,
				},
			}

			created, err := tc.PipelineRuns(defaultNamespace).Create(ctx, pr, metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("Failed to create PipelineRun: %v", err)
			}
			t.Cleanup(func() {
				_ = tc.PipelineRuns(defaultNamespace).Delete(ctx, created.Name, metav1.DeleteOptions{})
			})

			if err := wait.PollUntilContextTimeout(ctx, 1*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
				got, err := tc.PipelineRuns(defaultNamespace).Get(ctx, created.Name, metav1.GetOptions{})
				if err != nil {
					return false, err
				}
				_, hasResult := got.Annotations["results.tekton.dev/result"]
				_, hasRecord := got.Annotations["results.tekton.dev/record"]
				return hasResult && hasRecord, nil
			}); err != nil {
				t.Fatalf("Timed out waiting for Results annotations on PipelineRun: %v", err)
			}
		})
	}
}
