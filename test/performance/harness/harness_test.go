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

package main

import (
	"testing"

	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"github.com/tektoncd/results/test/performance/generator"
)

func testRunConfig() *RunConfig {
	return &RunConfig{
		Seed:           42,
		DatasetVersion: "test-v1",
		Count:          500,
		Namespaces:     50,
		ChildMin:       2,
		ChildMax:       15,
	}
}

func TestStagedPipelineRuns(t *testing.T) {
	g, err := newGenerator(testRunConfig(), false)
	if err != nil {
		t.Fatalf("newGenerator: %v", err)
	}
	inst, err := g.At(0)
	if err != nil {
		t.Fatalf("At(0): %v", err)
	}

	for _, updates := range []int{1, 2, 3} {
		stages := stagedPipelineRuns(inst.PipelineRun, updates)
		if got, want := len(stages), updates+1; got != want {
			t.Errorf("updates=%d: got %d stages, want %d", updates, got, want)
		}
		pending := stages[0]
		if pending.Status.CompletionTime != nil {
			t.Errorf("updates=%d: pending stage has completion time set", updates)
		}
		if len(pending.Status.Conditions) == 0 || pending.Status.Conditions[0].Reason != "Running" {
			t.Errorf("updates=%d: pending stage is not Running", updates)
		}
		terminal := stages[len(stages)-1]
		if terminal.Status.CompletionTime == nil {
			t.Errorf("updates=%d: terminal stage missing completion time", updates)
		}
	}
}

func TestUpdateCountForRange(t *testing.T) {
	for i := 0; i < 1000; i++ {
		if n := updateCountFor(i); n < 1 || n > 3 {
			t.Fatalf("updateCountFor(%d) = %d, out of [1,3]", i, n)
		}
	}
}

func TestSummaryStatus(t *testing.T) {
	tests := []struct {
		outcome generator.Outcome
		want    pb.RecordSummary_Status
	}{
		{generator.OutcomeSucceeded, pb.RecordSummary_SUCCESS},
		{generator.OutcomeFailed, pb.RecordSummary_FAILURE},
		{generator.OutcomeCancelled, pb.RecordSummary_CANCELLED},
		{generator.Outcome("other"), pb.RecordSummary_UNKNOWN},
	}
	for _, tt := range tests {
		if got := summaryStatus(tt.outcome); got != tt.want {
			t.Errorf("summaryStatus(%q) = %v, want %v", tt.outcome, got, tt.want)
		}
	}
}

func TestSplitWorkers(t *testing.T) {
	tests := []struct {
		total, read, write   int
		wantWriters, wantRds int
	}{
		{1, 3, 1, 1, 1},    // too few → one each
		{8, 3, 1, 2, 6},    // 1/4 writers
		{8, 1, 1, 4, 4},    // even
		{4, 0, 0, 2, 2},    // no ratio → even split
		{10, 9, 1, 1, 9},   // writers floored to at least 1
		{10, 1, 100, 9, 1}, // writers capped at total-1
	}
	for _, tt := range tests {
		w, r := splitWorkers(tt.total, tt.read, tt.write)
		if w != tt.wantWriters || r != tt.wantRds {
			t.Errorf("splitWorkers(%d,%d,%d) = (%d,%d), want (%d,%d)", tt.total, tt.read, tt.write, w, r, tt.wantWriters, tt.wantRds)
		}
		if w+r != max(tt.total, 2) && tt.total >= 2 {
			t.Errorf("splitWorkers(%d,...) sum = %d, want %d", tt.total, w+r, tt.total)
		}
	}
}

func TestListerPicker(t *testing.T) {
	clients := &APIClients{}
	if _, err := listerPicker("bogus", clients); err == nil {
		t.Error("listerPicker(bogus) = nil error, want error")
	}
	for _, transport := range []string{"grpc", "rest", "both"} {
		if _, err := listerPicker(transport, clients); err != nil {
			t.Errorf("listerPicker(%q) = %v", transport, err)
		}
	}
}

func TestSeedLiveUIDsDisjoint(t *testing.T) {
	cfg := testRunConfig()
	seed, err := newGenerator(cfg, false)
	if err != nil {
		t.Fatalf("seed generator: %v", err)
	}
	live, err := newGenerator(cfg, true)
	if err != nil {
		t.Fatalf("live generator: %v", err)
	}

	seen := map[string]bool{}
	for _, inst := range seed.Stream() {
		seen[inst.UID] = true
	}
	for _, inst := range live.Stream() {
		if seen[inst.UID] {
			t.Fatalf("live UID %q collides with seed range", inst.UID)
		}
	}
}
