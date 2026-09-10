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

package metrics

import (
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestLatencyStatsExact(t *testing.T) {
	r := NewLatencyRecorder()
	// 100 samples: 1ms..100ms, inserted out of order to exercise sorting.
	for i := 100; i >= 1; i-- {
		r.Observe(time.Duration(i) * time.Millisecond)
	}
	got := r.Snapshot()

	want := LatencyStats{
		Count: 100,
		Min:   1 * time.Millisecond,
		Max:   100 * time.Millisecond,
		Mean:  50500 * time.Microsecond, // (1+..+100)ms / 100 = 50.5ms
		P50:   51 * time.Millisecond,    // nearest-rank: index 50
		P90:   91 * time.Millisecond,
		P99:   100 * time.Millisecond,
	}
	if got != want {
		t.Errorf("Snapshot() = %+v, want %+v", got, want)
	}
}

func TestLatencyStatsEmpty(t *testing.T) {
	if got := NewLatencyRecorder().Snapshot(); got != (LatencyStats{}) {
		t.Errorf("empty Snapshot() = %+v, want zero value", got)
	}
}

func TestLatencyStatsSingle(t *testing.T) {
	r := NewLatencyRecorder()
	r.Observe(7 * time.Millisecond)
	got := r.Snapshot()
	want := LatencyStats{Count: 1, Min: 7 * time.Millisecond, Max: 7 * time.Millisecond, Mean: 7 * time.Millisecond, P50: 7 * time.Millisecond, P90: 7 * time.Millisecond, P99: 7 * time.Millisecond}
	if got != want {
		t.Errorf("single Snapshot() = %+v, want %+v", got, want)
	}
}

func TestLatencyMergeEqualsSingle(t *testing.T) {
	single := NewLatencyRecorder()
	a := NewLatencyRecorder()
	b := NewLatencyRecorder()
	for i := 1; i <= 200; i++ {
		d := time.Duration(i) * time.Microsecond
		single.Observe(d)
		if i%2 == 0 {
			a.Observe(d)
		} else {
			b.Observe(d)
		}
	}
	a.Merge(b)
	if got, want := a.Snapshot(), single.Snapshot(); got != want {
		t.Errorf("merged Snapshot() = %+v, want %+v", got, want)
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, "OK"},
		{"grpc not found", status.Error(codes.NotFound, "missing"), "NotFound"},
		{"grpc already exists", status.Error(codes.AlreadyExists, "dup"), "AlreadyExists"},
		{"plain error", errors.New("boom"), "Unknown"},
	}
	for _, tt := range tests {
		if got := Classify(tt.err); got != tt.want {
			t.Errorf("Classify(%s) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestErrorCounter(t *testing.T) {
	e := NewErrorCounter()
	e.Record(nil)
	e.Record(nil)
	e.Record(status.Error(codes.NotFound, "x"))
	e.Record(status.Error(codes.NotFound, "y"))
	e.Record(status.Error(codes.AlreadyExists, "z"))

	snap := e.Snapshot()
	if snap["OK"] != 2 || snap["NotFound"] != 2 || snap["AlreadyExists"] != 1 {
		t.Errorf("Snapshot() = %v", snap)
	}
	if got := e.Failures(); got != 3 {
		t.Errorf("Failures() = %d, want 3", got)
	}
}

func TestMetricSetSnapshot(t *testing.T) {
	m := NewMetricSet()
	m.Start()
	m.Observe("create_record", 10*time.Millisecond, nil)
	m.Observe("create_record", 20*time.Millisecond, status.Error(codes.Unavailable, "down"))
	m.Observe("update_record", 5*time.Millisecond, nil)
	m.Stop()

	snap := m.Snapshot()
	if len(snap.Ops) != 2 {
		t.Fatalf("ops = %d, want 2", len(snap.Ops))
	}
	// Sorted by name: create_record before update_record.
	if snap.Ops[0].Name != "create_record" || snap.Ops[1].Name != "update_record" {
		t.Errorf("ops not sorted: %s, %s", snap.Ops[0].Name, snap.Ops[1].Name)
	}
	if snap.TotalOps != 3 {
		t.Errorf("TotalOps = %d, want 3", snap.TotalOps)
	}
	if snap.TotalError != 1 {
		t.Errorf("TotalError = %d, want 1", snap.TotalError)
	}
	if snap.Ops[0].ErrorTotal != 1 {
		t.Errorf("create_record ErrorTotal = %d, want 1", snap.Ops[0].ErrorTotal)
	}
}

func TestMetricSetMerge(t *testing.T) {
	combined := NewMetricSet()
	a := NewMetricSet()
	b := NewMetricSet()
	for i := 1; i <= 50; i++ {
		d := time.Duration(i) * time.Millisecond
		combined.Observe("op", d, nil)
		if i%2 == 0 {
			a.Observe("op", d, nil)
		} else {
			b.Observe("op", d, nil)
		}
	}
	a.Merge(b)
	if got, want := a.Snapshot().Ops[0].Latency, combined.Snapshot().Ops[0].Latency; got != want {
		t.Errorf("merged latency = %+v, want %+v", got, want)
	}
}
