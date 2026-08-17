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

package reconciler

import (
	"testing"
	"time"

	v1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/ptr"
)

func TestIsManagedByAllowed(t *testing.T) {
	allowed := sets.New[string]("tekton.dev/pipeline")

	tests := []struct {
		name      string
		managedBy *string
		allowed   sets.Set[string]
		expected  bool
	}{
		{
			name:      "nil managedBy is allowed",
			managedBy: nil,
			allowed:   allowed,
			expected:  true,
		},
		{
			name:      "empty string managedBy is allowed",
			managedBy: ptr.To(""),
			allowed:   allowed,
			expected:  true,
		},
		{
			name:      "tekton.dev/pipeline is allowed",
			managedBy: ptr.To("tekton.dev/pipeline"),
			allowed:   allowed,
			expected:  true,
		},
		{
			name:      "custom controller is rejected",
			managedBy: ptr.To("custom-controller"),
			allowed:   allowed,
			expected:  false,
		},
		{
			name:      "custom controller is allowed when configured",
			managedBy: ptr.To("custom-controller"),
			allowed:   sets.New[string]("tekton.dev/pipeline", "custom-controller"),
			expected:  true,
		},
		{
			name:      "nil allowlist rejects custom controller",
			managedBy: ptr.To("custom-controller"),
			allowed:   nil,
			expected:  false,
		},
		{
			name:      "nil allowlist accepts tekton.dev/pipeline",
			managedBy: ptr.To("tekton.dev/pipeline"),
			allowed:   nil,
			expected:  true,
		},
		{
			name:      "nil allowlist accepts nil managedBy",
			managedBy: nil,
			allowed:   nil,
			expected:  true,
		},
		{
			name:      "whitespace-only managedBy is allowed",
			managedBy: ptr.To("  "),
			allowed:   allowed,
			expected:  true,
		},
		{
			name:      "trimmed managedBy matches allowed value",
			managedBy: ptr.To(" custom-controller "),
			allowed:   sets.New[string]("tekton.dev/pipeline", "custom-controller"),
			expected:  true,
		},
		{
			name:      "trimmed tekton.dev/pipeline is allowed with nil allowlist",
			managedBy: ptr.To(" tekton.dev/pipeline "),
			allowed:   nil,
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsManagedByAllowed(tt.managedBy, tt.allowed)
			if result != tt.expected {
				t.Errorf("IsManagedByAllowed() = %v, wanted %v", result, tt.expected)
			}
		})
	}
}

func TestParseManagedByValues(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		expected sets.Set[string]
	}{
		{
			name:     "empty string includes default",
			raw:      "",
			expected: sets.New[string]("tekton.dev/pipeline"),
		},
		{
			name:     "single value",
			raw:      "custom-controller",
			expected: sets.New[string]("tekton.dev/pipeline", "custom-controller"),
		},
		{
			name:     "multiple values with whitespace",
			raw:      "custom-controller, another-controller",
			expected: sets.New[string]("tekton.dev/pipeline", "custom-controller", "another-controller"),
		},
		{
			name:     "duplicate default value",
			raw:      "tekton.dev/pipeline",
			expected: sets.New[string]("tekton.dev/pipeline"),
		},
		{
			name:     "trailing comma ignored",
			raw:      "custom-controller,",
			expected: sets.New[string]("tekton.dev/pipeline", "custom-controller"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseManagedByValues(tt.raw)
			if !result.Equal(tt.expected) {
				t.Errorf("ParseManagedByValues(%q) = %v, wanted %v", tt.raw, result, tt.expected)
			}
		})
	}
}

func TestPipelineRunFilterFunc(t *testing.T) {
	defaultAllowed := sets.New[string]("tekton.dev/pipeline")

	tests := []struct {
		name     string
		allowed  sets.Set[string]
		obj      interface{}
		expected bool
	}{
		{
			name:     "nil managedBy, should match",
			allowed:  defaultAllowed,
			obj:      &v1.PipelineRun{ObjectMeta: metav1.ObjectMeta{Namespace: "default"}},
			expected: true,
		},
		{
			name:    "empty string managedBy, should match",
			allowed: defaultAllowed,
			obj: &v1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec:       v1.PipelineRunSpec{ManagedBy: ptr.To("")},
			},
			expected: true,
		},
		{
			name:    "tekton.dev/pipeline managedBy, should match",
			allowed: defaultAllowed,
			obj: &v1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec:       v1.PipelineRunSpec{ManagedBy: ptr.To("tekton.dev/pipeline")},
			},
			expected: true,
		},
		{
			name:    "custom controller, should not match",
			allowed: defaultAllowed,
			obj: &v1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec:       v1.PipelineRunSpec{ManagedBy: ptr.To("custom-controller")},
			},
			expected: false,
		},
		{
			name:    "custom controller with custom config, should match",
			allowed: sets.New[string]("tekton.dev/pipeline", "custom-controller"),
			obj: &v1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec:       v1.PipelineRunSpec{ManagedBy: ptr.To("custom-controller")},
			},
			expected: true,
		},
		{
			name:     "non-PipelineRun object, should not match",
			allowed:  defaultAllowed,
			obj:      &v1.TaskRun{ObjectMeta: metav1.ObjectMeta{Namespace: "default"}},
			expected: false,
		},
		{
			name:    "tombstone with allowed PipelineRun, should match",
			allowed: defaultAllowed,
			obj: cache.DeletedFinalStateUnknown{
				Key: "default/tombstone-pr",
				Obj: &v1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
					Spec:       v1.PipelineRunSpec{ManagedBy: ptr.To("tekton.dev/pipeline")},
				},
			},
			expected: true,
		},
		{
			name:    "tombstone with custom controller PipelineRun, should not match",
			allowed: defaultAllowed,
			obj: cache.DeletedFinalStateUnknown{
				Key: "default/tombstone-pr-custom",
				Obj: &v1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
					Spec:       v1.PipelineRunSpec{ManagedBy: ptr.To("custom-controller")},
				},
			},
			expected: false,
		},
		{
			name:    "tombstone with nil managedBy PipelineRun, should match",
			allowed: defaultAllowed,
			obj: cache.DeletedFinalStateUnknown{
				Key: "default/tombstone-pr-nil",
				Obj: &v1.PipelineRun{ObjectMeta: metav1.ObjectMeta{Namespace: "default"}},
			},
			expected: true,
		},
		{
			name:    "disallowed managedBy with finalizer but not deleting, should not match",
			allowed: defaultAllowed,
			obj: &v1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:  "default",
					Finalizers: []string{PipelineRunFinalizer},
				},
				Spec: v1.PipelineRunSpec{ManagedBy: ptr.To("custom-controller")},
			},
			expected: false,
		},
		{
			name:    "disallowed managedBy with finalizer and deleting, should match",
			allowed: defaultAllowed,
			obj: &v1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         "default",
					Finalizers:        []string{PipelineRunFinalizer},
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
				},
				Spec: v1.PipelineRunSpec{ManagedBy: ptr.To("custom-controller")},
			},
			expected: true,
		},
		{
			name:    "tombstone with disallowed managedBy but has Results finalizer, should match",
			allowed: defaultAllowed,
			obj: cache.DeletedFinalStateUnknown{
				Key: "default/tombstone-pr-finalizer",
				Obj: &v1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Finalizers:        []string{PipelineRunFinalizer},
						DeletionTimestamp: &metav1.Time{Time: time.Now()},
					},
					Spec: v1.PipelineRunSpec{ManagedBy: ptr.To("custom-controller")},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filterFunc := PipelineRunFilterFunc(tt.allowed)
			result := filterFunc(tt.obj)
			if result != tt.expected {
				t.Errorf("PipelineRunFilterFunc() = %v, wanted %v", result, tt.expected)
			}
		})
	}
}

func TestTaskRunFilterFunc(t *testing.T) {
	defaultAllowed := sets.New[string]("tekton.dev/pipeline")

	tests := []struct {
		name     string
		allowed  sets.Set[string]
		obj      interface{}
		expected bool
	}{
		{
			name:     "nil managedBy, should match",
			allowed:  defaultAllowed,
			obj:      &v1.TaskRun{ObjectMeta: metav1.ObjectMeta{Namespace: "default"}},
			expected: true,
		},
		{
			name:    "empty string managedBy, should match",
			allowed: defaultAllowed,
			obj: &v1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec:       v1.TaskRunSpec{ManagedBy: ptr.To("")},
			},
			expected: true,
		},
		{
			name:    "tekton.dev/pipeline managedBy, should match",
			allowed: defaultAllowed,
			obj: &v1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec:       v1.TaskRunSpec{ManagedBy: ptr.To("tekton.dev/pipeline")},
			},
			expected: true,
		},
		{
			name:    "custom controller, should not match",
			allowed: defaultAllowed,
			obj: &v1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec:       v1.TaskRunSpec{ManagedBy: ptr.To("custom-controller")},
			},
			expected: false,
		},
		{
			name:    "custom controller with custom config, should match",
			allowed: sets.New[string]("tekton.dev/pipeline", "custom-controller"),
			obj: &v1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec:       v1.TaskRunSpec{ManagedBy: ptr.To("custom-controller")},
			},
			expected: true,
		},
		{
			name:     "non-TaskRun object, should not match",
			allowed:  defaultAllowed,
			obj:      &v1.PipelineRun{ObjectMeta: metav1.ObjectMeta{Namespace: "default"}},
			expected: false,
		},
		{
			name:    "tombstone with allowed TaskRun, should match",
			allowed: defaultAllowed,
			obj: cache.DeletedFinalStateUnknown{
				Key: "default/tombstone-tr",
				Obj: &v1.TaskRun{
					ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
					Spec:       v1.TaskRunSpec{ManagedBy: ptr.To("tekton.dev/pipeline")},
				},
			},
			expected: true,
		},
		{
			name:    "tombstone with custom controller TaskRun, should not match",
			allowed: defaultAllowed,
			obj: cache.DeletedFinalStateUnknown{
				Key: "default/tombstone-tr-custom",
				Obj: &v1.TaskRun{
					ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
					Spec:       v1.TaskRunSpec{ManagedBy: ptr.To("custom-controller")},
				},
			},
			expected: false,
		},
		{
			name:    "tombstone with nil managedBy TaskRun, should match",
			allowed: defaultAllowed,
			obj: cache.DeletedFinalStateUnknown{
				Key: "default/tombstone-tr-nil",
				Obj: &v1.TaskRun{ObjectMeta: metav1.ObjectMeta{Namespace: "default"}},
			},
			expected: true,
		},
		{
			name:    "disallowed managedBy with finalizer but not deleting, should not match",
			allowed: defaultAllowed,
			obj: &v1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:  "default",
					Finalizers: []string{TaskRunFinalizer},
				},
				Spec: v1.TaskRunSpec{ManagedBy: ptr.To("custom-controller")},
			},
			expected: false,
		},
		{
			name:    "disallowed managedBy with finalizer and deleting, should match",
			allowed: defaultAllowed,
			obj: &v1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         "default",
					Finalizers:        []string{TaskRunFinalizer},
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
				},
				Spec: v1.TaskRunSpec{ManagedBy: ptr.To("custom-controller")},
			},
			expected: true,
		},
		{
			name:    "tombstone with disallowed managedBy but has Results finalizer, should match",
			allowed: defaultAllowed,
			obj: cache.DeletedFinalStateUnknown{
				Key: "default/tombstone-tr-finalizer",
				Obj: &v1.TaskRun{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Finalizers:        []string{TaskRunFinalizer},
						DeletionTimestamp: &metav1.Time{Time: time.Now()},
					},
					Spec: v1.TaskRunSpec{ManagedBy: ptr.To("custom-controller")},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filterFunc := TaskRunFilterFunc(tt.allowed)
			result := filterFunc(tt.obj)
			if result != tt.expected {
				t.Errorf("TaskRunFilterFunc() = %v, wanted %v", result, tt.expected)
			}
		})
	}
}
