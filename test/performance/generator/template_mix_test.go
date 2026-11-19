/*
Copyright 2026 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package generator

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"testing"
	"testing/fstest"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

// findTemplate returns the loaded template with the given id, or nil.
func findTemplate(ts *TemplateSet, id string) *Template {
	for _, t := range ts.Templates {
		if t.ID == id {
			return t
		}
	}
	return nil
}

func TestLoadTaskRunsDirectory(t *testing.T) {
	ts, err := LoadTemplates(embeddedTemplates, "templates")
	if err != nil {
		t.Fatalf("LoadTemplates() = %v", err)
	}
	tmpl := findTemplate(ts, "fbc-builder")
	if tmpl == nil {
		t.Fatalf("fbc-builder template not loaded")
	}

	if got, want := len(tmpl.taskRuns), 10; got != want {
		t.Fatalf("fbc-builder skeleton count = %d, want %d", got, want)
	}

	weights := map[string]float64{}
	for _, s := range tmpl.taskRuns {
		if s.tr == nil {
			t.Errorf("skeleton %q has nil TaskRun", s.name)
		}
		weights[s.name] = s.weight
	}
	if got, want := weights["build"], 4.0; got != want {
		t.Errorf("build weight = %v, want %v", got, want)
	}
	for name, w := range weights {
		if name == "build" {
			continue
		}
		if w != 1.0 {
			t.Errorf("%s weight = %v, want 1.0", name, w)
		}
	}
	if got, want := tmpl.taskRunWeight, 13.0; got != want {
		t.Errorf("total skeleton weight = %v, want %v", got, want)
	}
}

func TestLoadSingleTaskRunFile(t *testing.T) {
	ts, err := LoadTemplates(embeddedTemplates, "templates")
	if err != nil {
		t.Fatalf("LoadTemplates() = %v", err)
	}
	tmpl := findTemplate(ts, "sample")
	if tmpl == nil {
		t.Fatalf("sample template not loaded")
	}
	if got, want := len(tmpl.taskRuns), 1; got != want {
		t.Fatalf("sample skeleton count = %d, want %d", got, want)
	}
	if got, want := tmpl.taskRuns[0].name, "taskrun"; got != want {
		t.Errorf("skeleton name = %q, want %q", got, want)
	}
}

func TestLoadSimpleTemplate(t *testing.T) {
	ts, err := LoadTemplates(embeddedTemplates, "templates/default")
	if err != nil {
		t.Fatalf("LoadTemplates() = %v", err)
	}
	tmpl := findTemplate(ts, "simple")
	if tmpl == nil {
		t.Fatalf("simple template not loaded")
	}
	if got, want := len(tmpl.taskRuns), 2; got != want {
		t.Fatalf("simple skeleton count = %d, want %d", got, want)
	}
	// Skeletons load in sorted filename order; the build-container weight override
	// from template.yaml must be applied while clone-repository keeps the default.
	want := []childSkeleton{{name: "build-container", weight: 3}, {name: "clone-repository", weight: 1}}
	for i, w := range want {
		if got := tmpl.taskRuns[i]; got.name != w.name || got.weight != w.weight {
			t.Errorf("skeleton[%d] = {name:%q weight:%v}, want {name:%q weight:%v}", i, got.name, got.weight, w.name, w.weight)
		}
	}
}

func TestLoadDefaultTaskRunFallback(t *testing.T) {
	fsys := fstest.MapFS{
		"t/only/pipelinerun.yaml": &fstest.MapFile{
			Data: []byte("apiVersion: tekton.dev/v1\nkind: PipelineRun\nmetadata:\n  name: only\n"),
		},
	}
	ts, err := LoadTemplates(fsys, "t")
	if err != nil {
		t.Fatalf("LoadTemplates() = %v", err)
	}
	tmpl := findTemplate(ts, "only")
	if tmpl == nil {
		t.Fatalf("only template not loaded")
	}
	if got, want := len(tmpl.taskRuns), 1; got != want {
		t.Fatalf("skeleton count = %d, want %d", got, want)
	}
	if got, want := tmpl.taskRuns[0].name, "default"; got != want {
		t.Errorf("skeleton name = %q, want %q", got, want)
	}
}

func TestChildSkeletonSizeMixPresent(t *testing.T) {
	cfg := testConfig(t, 1)
	g := mustGenerator(t, cfg)
	inst, err := g.At(0)
	if err != nil {
		t.Fatalf("At(0) = %v", err)
	}
	if len(inst.TaskRuns) < 2 {
		t.Fatalf("instance has %d TaskRuns, need at least 2 to prove a mix", len(inst.TaskRuns))
	}
	sizes := map[int]bool{}
	for _, tr := range inst.TaskRuns {
		b, err := json.Marshal(tr)
		if err != nil {
			t.Fatalf("marshal TaskRun: %v", err)
		}
		sizes[len(b)] = true
	}
	if len(sizes) < 2 {
		t.Errorf("expected at least 2 distinct TaskRun body sizes, got %d", len(sizes))
	}
}

func TestPickTaskRunWeightedDistribution(t *testing.T) {
	ts, err := LoadTemplates(embeddedTemplates, "templates")
	if err != nil {
		t.Fatalf("LoadTemplates() = %v", err)
	}
	tmpl := findTemplate(ts, "fbc-builder")
	if tmpl == nil {
		t.Fatalf("fbc-builder template not loaded")
	}

	byPtr := map[*childSkeleton]string{}
	for i := range tmpl.taskRuns {
		byPtr[&tmpl.taskRuns[i]] = tmpl.taskRuns[i].name
	}

	const n = 200000
	rng := rand.New(rand.NewPCG(42, 7)) //nolint:gosec // deterministic test
	build := 0
	for i := 0; i < n; i++ {
		tr := tmpl.pickTaskRun(rng)
		for j := range tmpl.taskRuns {
			if tmpl.taskRuns[j].tr == tr {
				if tmpl.taskRuns[j].name == "build" {
					build++
				}
				break
			}
		}
	}

	got := float64(build) / float64(n)
	want := 4.0 / 13.0
	if math.Abs(got-want) > 0.02 {
		t.Errorf("build share = %.4f, want %.4f ± 0.02", got, want)
	}
}

func TestPickTaskRunSingleSkeletonDrawsNoRng(t *testing.T) {
	tmpl := &Template{taskRuns: []childSkeleton{{name: "only", weight: 1, tr: defaultTaskRunSkeleton()}}, taskRunWeight: 1}

	drew := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // deterministic test
	before := drew.Uint64()

	check := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // deterministic test
	if got := tmpl.pickTaskRun(check); got != tmpl.taskRuns[0].tr {
		t.Fatalf("pickTaskRun returned unexpected skeleton")
	}
	// A single-skeleton template must not consume any rng value, so the next draw
	// still matches the first value drawn from an untouched generator.
	if after := check.Uint64(); after != before {
		t.Errorf("single-skeleton pick consumed rng: next draw %d != %d", after, before)
	}
}

func TestPickTaskRunWithZeroWeights(t *testing.T) {
	tests := []struct {
		name      string
		skeletons []childSkeleton
		wantName  string
	}{
		{
			name: "zero weight skipped",
			skeletons: []childSkeleton{
				{name: "zero", weight: 0, tr: &tektonv1.TaskRun{Spec: tektonv1.TaskRunSpec{TaskRef: &tektonv1.TaskRef{Name: "zero"}}}},
				{name: "one", weight: 1, tr: &tektonv1.TaskRun{Spec: tektonv1.TaskRunSpec{TaskRef: &tektonv1.TaskRef{Name: "one"}}}},
			},
			wantName: "one",
		},
		{
			name: "mixed weights with zero",
			skeletons: []childSkeleton{
				{name: "a", weight: 2, tr: &tektonv1.TaskRun{Spec: tektonv1.TaskRunSpec{TaskRef: &tektonv1.TaskRef{Name: "a"}}}},
				{name: "b", weight: 0, tr: &tektonv1.TaskRun{Spec: tektonv1.TaskRunSpec{TaskRef: &tektonv1.TaskRef{Name: "b"}}}},
				{name: "c", weight: 3, tr: &tektonv1.TaskRun{Spec: tektonv1.TaskRunSpec{TaskRef: &tektonv1.TaskRef{Name: "c"}}}},
			},
			wantName: "c",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var totalWeight float64
			for _, s := range tc.skeletons {
				totalWeight += s.weight
			}
			tmpl := &Template{taskRuns: tc.skeletons, taskRunWeight: totalWeight}

			rng := rand.New(rand.NewPCG(42, 7)) //nolint:gosec // deterministic test
			counts := map[string]int{}
			const n = 1000
			for i := 0; i < n; i++ {
				tr := tmpl.pickTaskRun(rng)
				for j := range tc.skeletons {
					if tc.skeletons[j].tr == tr {
						counts[tc.skeletons[j].name]++
						break
					}
				}
			}
			if counts["zero"] > 0 {
				t.Errorf("zero-weight skeleton was picked %d times", counts["zero"])
			}
			if tc.name == "zero weight skipped" && counts[tc.wantName] != n {
				t.Errorf("expected %q to be picked %d times, got %d", tc.wantName, n, counts[tc.wantName])
			}
		})
	}
}

func TestPickTaskRunAlwaysReturnsLastOnEdge(t *testing.T) {
	skeletons := []childSkeleton{
		{name: "a", weight: 1, tr: &tektonv1.TaskRun{Spec: tektonv1.TaskRunSpec{TaskRef: &tektonv1.TaskRef{Name: "a"}}}},
		{name: "b", weight: 1, tr: &tektonv1.TaskRun{Spec: tektonv1.TaskRunSpec{TaskRef: &tektonv1.TaskRef{Name: "b"}}}},
		{name: "c", weight: 1, tr: &tektonv1.TaskRun{Spec: tektonv1.TaskRunSpec{TaskRef: &tektonv1.TaskRef{Name: "c"}}}},
	}
	tmpl := &Template{taskRuns: skeletons, taskRunWeight: 3}

	const n = 10000
	rng := rand.New(rand.NewPCG(42, 7)) //nolint:gosec // deterministic test
	gotLast := false
	for i := 0; i < n; i++ {
		tr := tmpl.pickTaskRun(rng)
		if tr == skeletons[len(skeletons)-1].tr {
			gotLast = true
			break
		}
	}
	if !gotLast {
		t.Error("expected last skeleton to be returned at least once over many draws")
	}
}

func TestLoadChildSkeletonsPrecedence(t *testing.T) {
	t.Run("taskruns dir wins over taskrun.yaml", func(t *testing.T) {
		fsys := fstest.MapFS{
			"t/both/pipelinerun.yaml": &fstest.MapFile{
				Data: []byte("apiVersion: tekton.dev/v1\nkind: PipelineRun\nmetadata:\n  name: both\n"),
			},
			"t/both/taskrun.yaml": &fstest.MapFile{
				Data: []byte("apiVersion: tekton.dev/v1\nkind: TaskRun\nmetadata:\n  name: single\nspec:\n  taskRef:\n    name: single-task\n"),
			},
			"t/both/taskruns/multi-a.yaml": &fstest.MapFile{
				Data: []byte("apiVersion: tekton.dev/v1\nkind: TaskRun\nmetadata:\n  name: multi-a\nspec:\n  taskRef:\n    name: task-a\n"),
			},
			"t/both/taskruns/multi-b.yaml": &fstest.MapFile{
				Data: []byte("apiVersion: tekton.dev/v1\nkind: TaskRun\nmetadata:\n  name: multi-b\nspec:\n  taskRef:\n    name: task-b\n"),
			},
		}
		ts, err := LoadTemplates(fsys, "t")
		if err != nil {
			t.Fatalf("LoadTemplates() = %v", err)
		}
		tmpl := findTemplate(ts, "both")
		if tmpl == nil {
			t.Fatalf("both template not loaded")
		}
		if got, want := len(tmpl.taskRuns), 2; got != want {
			t.Fatalf("skeleton count = %d, want %d (taskruns/ should win)", got, want)
		}
		names := []string{tmpl.taskRuns[0].name, tmpl.taskRuns[1].name}
		if names[0] != "multi-a" || names[1] != "multi-b" {
			t.Errorf("skeleton names = %v, want [multi-a multi-b] (taskruns/ should win)", names)
		}
	})

	t.Run("empty taskruns dir falls back to taskrun.yaml", func(t *testing.T) {
		fsys := fstest.MapFS{
			"t/empty/pipelinerun.yaml": &fstest.MapFile{
				Data: []byte("apiVersion: tekton.dev/v1\nkind: PipelineRun\nmetadata:\n  name: empty\n"),
			},
			"t/empty/taskrun.yaml": &fstest.MapFile{
				Data: []byte("apiVersion: tekton.dev/v1\nkind: TaskRun\nmetadata:\n  name: fallback\nspec:\n  taskRef:\n    name: fallback-task\n"),
			},
			"t/empty/taskruns/readme.txt": &fstest.MapFile{
				Data: []byte("not a yaml file"),
			},
		}
		ts, err := LoadTemplates(fsys, "t")
		if err != nil {
			t.Fatalf("LoadTemplates() = %v", err)
		}
		tmpl := findTemplate(ts, "empty")
		if tmpl == nil {
			t.Fatalf("empty template not loaded")
		}
		if got, want := len(tmpl.taskRuns), 1; got != want {
			t.Fatalf("skeleton count = %d, want %d", got, want)
		}
		if got, want := tmpl.taskRuns[0].name, "taskrun"; got != want {
			t.Errorf("skeleton name = %q, want %q", got, want)
		}
	})
}

func TestLoadChildSkeletonsSortedOrder(t *testing.T) {
	fsys := fstest.MapFS{
		"t/sorted/pipelinerun.yaml": &fstest.MapFile{
			Data: []byte("apiVersion: tekton.dev/v1\nkind: PipelineRun\nmetadata:\n  name: sorted\n"),
		},
		"t/sorted/taskruns/z-last.yaml": &fstest.MapFile{
			Data: []byte("apiVersion: tekton.dev/v1\nkind: TaskRun\nmetadata:\n  name: z\nspec:\n  taskRef:\n    name: z-task\n"),
		},
		"t/sorted/taskruns/a-first.yaml": &fstest.MapFile{
			Data: []byte("apiVersion: tekton.dev/v1\nkind: TaskRun\nmetadata:\n  name: a\nspec:\n  taskRef:\n    name: a-task\n"),
		},
		"t/sorted/taskruns/m-middle.yaml": &fstest.MapFile{
			Data: []byte("apiVersion: tekton.dev/v1\nkind: TaskRun\nmetadata:\n  name: m\nspec:\n  taskRef:\n    name: m-task\n"),
		},
	}
	ts, err := LoadTemplates(fsys, "t")
	if err != nil {
		t.Fatalf("LoadTemplates() = %v", err)
	}
	tmpl := findTemplate(ts, "sorted")
	if tmpl == nil {
		t.Fatalf("sorted template not loaded")
	}
	if got, want := len(tmpl.taskRuns), 3; got != want {
		t.Fatalf("skeleton count = %d, want %d", got, want)
	}
	names := []string{tmpl.taskRuns[0].name, tmpl.taskRuns[1].name, tmpl.taskRuns[2].name}
	want := []string{"a-first", "m-middle", "z-last"}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("skeleton[%d].name = %q, want %q (should be sorted by filename)", i, names[i], want[i])
		}
	}
}

func TestLoadChildSkeletonsMalformedYAML(t *testing.T) {
	tests := []struct {
		name    string
		fsys    fstest.MapFS
		wantErr string
	}{
		{
			name: "invalid yaml in taskruns dir",
			fsys: fstest.MapFS{
				"t/bad/pipelinerun.yaml": &fstest.MapFile{
					Data: []byte("apiVersion: tekton.dev/v1\nkind: PipelineRun\nmetadata:\n  name: bad\n"),
				},
				"t/bad/taskruns/broken.yaml": &fstest.MapFile{
					Data: []byte("this is not valid yaml: [[["),
				},
			},
			wantErr: "parsing t/bad/taskruns/broken.yaml",
		},
		{
			name: "invalid yaml in single taskrun.yaml",
			fsys: fstest.MapFS{
				"t/bad/pipelinerun.yaml": &fstest.MapFile{
					Data: []byte("apiVersion: tekton.dev/v1\nkind: PipelineRun\nmetadata:\n  name: bad\n"),
				},
				"t/bad/taskrun.yaml": &fstest.MapFile{
					Data: []byte("not: [valid: yaml"),
				},
			},
			wantErr: "parsing t/bad/taskrun.yaml",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadTemplates(tc.fsys, "t")
			if err == nil {
				t.Fatal("LoadTemplates() succeeded, want error")
			}
			if tc.wantErr != "" && !contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestMultiSkeletonOrderIndependence(t *testing.T) {
	cfg := testConfig(t, 100)
	g := mustGenerator(t, cfg)

	target := 50
	got1, err := g.At(target)
	if err != nil {
		t.Fatalf("At(%d) = %v", target, err)
	}

	for i := 0; i < cfg.Count; i++ {
		if i == target {
			continue
		}
		if _, err := g.At(i); err != nil {
			t.Fatalf("At(%d) = %v", i, err)
		}
	}

	got2, err := g.At(target)
	if err != nil {
		t.Fatalf("At(%d) second call = %v", target, err)
	}

	if string(marshalInstance(t, got1)) != string(marshalInstance(t, got2)) {
		t.Errorf("At(%d) returned different result after computing other indices", target)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
