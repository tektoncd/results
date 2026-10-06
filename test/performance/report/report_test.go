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

package report

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tektoncd/results/test/performance/metrics"
)

func sampleSnapshot() metrics.Snapshot {
	m := metrics.NewMetricSet()
	m.Start()
	for i := 1; i <= 100; i++ {
		m.Observe("create_record", time.Duration(i)*time.Millisecond, nil)
	}
	m.Observe("create_record", 5*time.Millisecond, status.Error(codes.Unavailable, "down"))
	m.Stop()
	snap := m.Snapshot()
	// Pin a deterministic duration so throughput is stable in assertions.
	snap.Duration = 2 * time.Second
	return snap
}

func TestNewReportShape(t *testing.T) {
	snap := sampleSnapshot()
	meta := Meta{GitCommit: "abc123", DatasetVersion: "seed-small-v1", Tier: "tier1", Mode: "store", DBBackend: "local"}
	cfg := Config{Count: 100, Concurrency: 8, Transport: "grpc", Seed: 42}

	r := New(meta, cfg, snap)

	if r.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", r.SchemaVersion, SchemaVersion)
	}
	if r.Meta.DurationMS != 2000 {
		t.Errorf("DurationMS = %d, want 2000", r.Meta.DurationMS)
	}
	op, ok := r.Metrics.ByOp["create_record"]
	if !ok {
		t.Fatal("missing create_record op report")
	}
	if op.Count != 101 {
		t.Errorf("Count = %d, want 101", op.Count)
	}
	if op.Errors != 1 || op.ErrorCodes["Unavailable"] != 1 {
		t.Errorf("errors = %d, codes = %v", op.Errors, op.ErrorCodes)
	}
	// 101 ops over 2s.
	if want := 101.0 / 2.0; r.Metrics.ThroughputPerSec != want {
		t.Errorf("ThroughputPerSec = %f, want %f", r.Metrics.ThroughputPerSec, want)
	}
}

func TestReportRoundTrip(t *testing.T) {
	r := New(
		Meta{GitCommit: "deadbeef", Tier: "tier1", Mode: "mixed"},
		Config{Count: 10, Concurrency: 2},
		sampleSnapshot(),
	)

	var buf bytes.Buffer
	if err := Write(&buf, r); err != nil {
		t.Fatalf("Write() = %v", err)
	}
	got, err := Read(&buf)
	if err != nil {
		t.Fatalf("Read() = %v", err)
	}
	if diff := cmp.Diff(r, got); diff != "" {
		t.Errorf("round-trip mismatch (-want +got):\n%s", diff)
	}
}

func TestReportRequiredKeys(t *testing.T) {
	r := New(Meta{Mode: "query"}, Config{}, sampleSnapshot())
	var buf bytes.Buffer
	if err := Write(&buf, r); err != nil {
		t.Fatalf("Write() = %v", err)
	}
	var generic map[string]any
	if err := json.Unmarshal(buf.Bytes(), &generic); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"schema_version", "meta", "config", "metrics"} {
		if _, ok := generic[key]; !ok {
			t.Errorf("report missing top-level key %q", key)
		}
	}
	metricsObj := generic["metrics"].(map[string]any)
	for _, key := range []string{"throughput_per_sec", "total_ops", "total_errors", "by_op"} {
		if _, ok := metricsObj[key]; !ok {
			t.Errorf("metrics missing key %q", key)
		}
	}
}
