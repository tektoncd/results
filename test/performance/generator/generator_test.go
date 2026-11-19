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
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	recordutil "github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	resultutil "github.com/tektoncd/results/pkg/api/server/v1alpha2/result"
)

var (
	seedSpace = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	liveSpace = uuid.MustParse("22222222-2222-2222-2222-222222222222")
)

func testConfig(t *testing.T, count int) Config {
	t.Helper()
	namespaces := make([]string, 50)
	for i := range namespaces {
		namespaces[i] = fmt.Sprintf("ns-%02d", i)
	}
	return Config{
		Seed:       42,
		Version:    "test-v1",
		Count:      count,
		Namespaces: namespaces,
		LabelPool: LabelPool{Keys: map[string]LabelValues{
			"appstudio.openshift.io/component": {
				Values:   []string{"frontend", "backend", "api", "db", "cache", "worker", "ingest", "reporting"},
				HotCount: 2,
			},
			"pipelinesascode.tekton.dev/event-type": {
				Values:   []string{"push", "pull_request"},
				HotCount: 1,
			},
		}},
		Outcomes: DefaultOutcomeRatios(),
		TimeWindow: TimeWindow{
			Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
		},
		UIDs:          UIDPartition{Space: seedSpace, Offset: 0},
		ChildTaskRuns: IntRange{Min: 2, Max: 15},
	}
}

func mustGenerator(t *testing.T, cfg Config) *Generator {
	t.Helper()
	g, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	return g
}

func marshalInstance(t *testing.T, inst *Instance) []byte {
	t.Helper()
	b, err := json.Marshal(struct {
		PR interface{}
		TR interface{}
	}{inst.PipelineRun, inst.TaskRuns})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestGeneratorDeterministic(t *testing.T) {
	cfg := testConfig(t, 200)
	g1 := mustGenerator(t, cfg)
	g2 := mustGenerator(t, cfg)

	for i := 0; i < cfg.Count; i++ {
		a, err := g1.At(i)
		if err != nil {
			t.Fatalf("g1.At(%d) = %v", i, err)
		}
		b, err := g2.At(i)
		if err != nil {
			t.Fatalf("g2.At(%d) = %v", i, err)
		}
		if got, want := string(marshalInstance(t, a)), string(marshalInstance(t, b)); got != want {
			t.Fatalf("instance %d not byte-identical across runs", i)
		}
	}

	d1, err := g1.Definition()
	if err != nil {
		t.Fatalf("Definition() = %v", err)
	}
	d2, err := g2.Definition()
	if err != nil {
		t.Fatalf("Definition() = %v", err)
	}
	if d1.ContentHash != d2.ContentHash {
		t.Errorf("ContentHash mismatch: %s != %s", d1.ContentHash, d2.ContentHash)
	}
	if d1.ContentHash == "" {
		t.Error("ContentHash is empty")
	}
}

func TestAtMatchesStream(t *testing.T) {
	cfg := testConfig(t, 100)
	g := mustGenerator(t, cfg)
	for i, inst := range g.Stream() {
		at, err := g.At(i)
		if err != nil {
			t.Fatalf("At(%d) = %v", i, err)
		}
		if got, want := string(marshalInstance(t, at)), string(marshalInstance(t, inst)); got != want {
			t.Fatalf("At(%d) != Stream()[%d]", i, i)
		}
	}
}

func TestNameSafety(t *testing.T) {
	cfg := testConfig(t, 500)
	g := mustGenerator(t, cfg)
	for _, inst := range g.Stream() {
		if _, _, err := resultutil.ParseName(inst.ResultName); err != nil {
			t.Fatalf("invalid result name %q: %v", inst.ResultName, err)
		}
		if _, _, _, err := recordutil.ParseName(inst.RecordName); err != nil {
			t.Fatalf("invalid record name %q: %v", inst.RecordName, err)
		}
		for _, cr := range inst.ChildRecords {
			if _, _, _, err := recordutil.ParseName(cr); err != nil {
				t.Fatalf("invalid child record name %q: %v", cr, err)
			}
		}
	}
}

func TestDistribution(t *testing.T) {
	const n = 6000
	cfg := testConfig(t, n)
	g := mustGenerator(t, cfg)
	d, err := g.Definition()
	if err != nil {
		t.Fatalf("Definition() = %v", err)
	}

	// Outcome ratios within tolerance of the configured 85/10/5 split.
	assertRatio(t, "succeeded", d.Outcomes["succeeded"], n, 0.85, 0.03)
	assertRatio(t, "failed", d.Outcomes["failed"], n, 0.10, 0.03)
	assertRatio(t, "cancelled", d.Outcomes["cancelled"], n, 0.05, 0.03)

	// Every namespace in the pool is exercised.
	if len(d.PerNamespace) != len(cfg.Namespaces) {
		t.Errorf("namespaces used = %d, want %d", len(d.PerNamespace), len(cfg.Namespaces))
	}

	// Hot label values dominate the long tail.
	comp := d.PerLabel["appstudio.openshift.io/component"]
	hot := comp["frontend"] + comp["backend"]
	tail := 0
	for v, c := range comp {
		if v != "frontend" && v != "backend" {
			tail += c
		}
	}
	if hot <= tail {
		t.Errorf("expected hot component values to dominate: hot=%d tail=%d", hot, tail)
	}
}

func TestUIDPartitionsDisjoint(t *testing.T) {
	const n = 1000
	seedCfg := testConfig(t, n)
	liveCfg := testConfig(t, n)
	liveCfg.UIDs = UIDPartition{Space: liveSpace, Offset: 1 << 32}

	seen := map[string]bool{}
	for _, inst := range mustGenerator(t, seedCfg).Stream() {
		seen[inst.UID] = true
		for _, cr := range inst.ChildRecords {
			seen[cr] = true
		}
	}
	for _, inst := range mustGenerator(t, liveCfg).Stream() {
		if seen[inst.UID] {
			t.Fatalf("live UID %q collides with seed range", inst.UID)
		}
	}
}

func TestDefinitionRoundTrip(t *testing.T) {
	cfg := testConfig(t, 50)
	g := mustGenerator(t, cfg)
	d, err := g.Definition()
	if err != nil {
		t.Fatalf("Definition() = %v", err)
	}

	var buf []byte
	buf, err = json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	got := &DatasetDefinition{}
	if err := json.Unmarshal(buf, got); err != nil {
		t.Fatalf("unmarshal definition: %v", err)
	}
	if got.ContentHash != d.ContentHash || got.Count != d.Count {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
}

func assertRatio(t *testing.T, name string, got, total int, want, tol float64) {
	t.Helper()
	ratio := float64(got) / float64(total)
	if ratio < want-tol || ratio > want+tol {
		t.Errorf("%s ratio = %.3f, want %.3f ± %.2f", name, ratio, want, tol)
	}
}
