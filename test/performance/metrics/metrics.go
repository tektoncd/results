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

// Package metrics collects per-operation latency and error statistics for the
// performance benchmark harness. Percentiles are computed exactly from the
// recorded samples (no external histogram dependency); per-worker recorders can
// be merged to avoid lock contention on the hot path.
package metrics

import (
	"maps"
	"slices"
	"sort"
	"sync"
	"time"

	"google.golang.org/grpc/status"
)

// LatencyRecorder accumulates operation latencies and computes exact
// percentiles on demand.
type LatencyRecorder struct {
	mu      sync.Mutex
	samples []time.Duration
}

// NewLatencyRecorder returns an empty recorder.
func NewLatencyRecorder() *LatencyRecorder { return &LatencyRecorder{} }

// Observe records a single latency sample.
func (r *LatencyRecorder) Observe(d time.Duration) {
	r.mu.Lock()
	r.samples = append(r.samples, d)
	r.mu.Unlock()
}

// Merge folds another recorder's samples into r.
func (r *LatencyRecorder) Merge(o *LatencyRecorder) {
	o.mu.Lock()
	defer o.mu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.samples = append(r.samples, o.samples...)
}

// LatencyStats is a computed summary of a set of latency samples.
type LatencyStats struct {
	Count int64
	Min   time.Duration
	Max   time.Duration
	Mean  time.Duration
	P50   time.Duration
	P90   time.Duration
	P99   time.Duration
}

// Snapshot sorts the samples once and computes the summary statistics.
func (r *LatencyRecorder) Snapshot() LatencyStats {
	r.mu.Lock()
	sorted := make([]time.Duration, len(r.samples))
	copy(sorted, r.samples)
	r.mu.Unlock()

	if len(sorted) == 0 {
		return LatencyStats{}
	}
	slices.Sort(sorted)

	var sum time.Duration
	for _, d := range sorted {
		sum += d
	}
	return LatencyStats{
		Count: int64(len(sorted)),
		Min:   sorted[0],
		Max:   sorted[len(sorted)-1],
		Mean:  sum / time.Duration(len(sorted)),
		P50:   percentile(sorted, 0.50),
		P90:   percentile(sorted, 0.90),
		P99:   percentile(sorted, 0.99),
	}
}

// percentile returns the p-quantile (p in [0,1]) using nearest-rank on a sorted
// slice. sorted must be non-empty.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 1 {
		return sorted[0]
	}
	rank := int(p * float64(len(sorted)))
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}

// ErrorCounter tallies operation results by gRPC status code (OK for success).
type ErrorCounter struct {
	mu     sync.Mutex
	byCode map[string]int64
}

// NewErrorCounter returns an empty counter.
func NewErrorCounter() *ErrorCounter { return &ErrorCounter{byCode: map[string]int64{}} }

// Record classifies err by gRPC status code and increments the tally. A nil err
// is counted as "OK".
func (e *ErrorCounter) Record(err error) { e.RecordCode(Classify(err)) }

// RecordCode increments the tally for an explicit code string.
func (e *ErrorCounter) RecordCode(code string) {
	e.mu.Lock()
	e.byCode[code]++
	e.mu.Unlock()
}

// Merge folds another counter into e.
func (e *ErrorCounter) Merge(o *ErrorCounter) {
	o.mu.Lock()
	defer o.mu.Unlock()
	e.mu.Lock()
	defer e.mu.Unlock()
	for k, v := range o.byCode {
		e.byCode[k] += v
	}
}

// Snapshot returns a copy of the per-code tallies.
func (e *ErrorCounter) Snapshot() map[string]int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make(map[string]int64, len(e.byCode))
	maps.Copy(out, e.byCode)
	return out
}

// Failures returns the number of non-OK results.
func (e *ErrorCounter) Failures() int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	var total int64
	for k, v := range e.byCode {
		if k != "OK" {
			total += v
		}
	}
	return total
}

// Classify maps an error to a gRPC status code string. Nil maps to "OK";
// non-status errors map to "Unknown".
func Classify(err error) string {
	return status.Code(err).String()
}

// OpMetrics bundles the latency recorder and error counter for one operation.
type OpMetrics struct {
	Latency *LatencyRecorder
	Errors  *ErrorCounter
}

// NewOpMetrics returns an initialized OpMetrics.
func NewOpMetrics() *OpMetrics {
	return &OpMetrics{Latency: NewLatencyRecorder(), Errors: NewErrorCounter()}
}

// Merge folds another OpMetrics into m.
func (m *OpMetrics) Merge(o *OpMetrics) {
	m.Latency.Merge(o.Latency)
	m.Errors.Merge(o.Errors)
}

// MetricSet holds per-operation metrics for a benchmark run.
type MetricSet struct {
	mu    sync.Mutex
	ops   map[string]*OpMetrics
	start time.Time
	end   time.Time
}

// NewMetricSet returns an initialized MetricSet.
func NewMetricSet() *MetricSet { return &MetricSet{ops: map[string]*OpMetrics{}} }

// Start marks the beginning of the measured window.
func (m *MetricSet) Start() {
	m.mu.Lock()
	m.start = time.Now()
	m.mu.Unlock()
}

// Stop marks the end of the measured window.
func (m *MetricSet) Stop() {
	m.mu.Lock()
	m.end = time.Now()
	m.mu.Unlock()
}

// Op returns the OpMetrics for name, creating it on first use.
func (m *MetricSet) Op(name string) *OpMetrics {
	m.mu.Lock()
	defer m.mu.Unlock()
	op, ok := m.ops[name]
	if !ok {
		op = NewOpMetrics()
		m.ops[name] = op
	}
	return op
}

// Observe is a convenience that records a latency and result for an operation.
func (m *MetricSet) Observe(name string, d time.Duration, err error) {
	op := m.Op(name)
	op.Latency.Observe(d)
	op.Errors.Record(err)
}

// Merge folds another MetricSet's operations into m, preserving m's window.
func (m *MetricSet) Merge(o *MetricSet) {
	o.mu.Lock()
	names := make([]string, 0, len(o.ops))
	src := make(map[string]*OpMetrics, len(o.ops))
	for k, v := range o.ops {
		names = append(names, k)
		src[k] = v
	}
	o.mu.Unlock()
	for _, name := range names {
		m.Op(name).Merge(src[name])
	}
}

// Duration returns the measured wall-clock window.
func (m *MetricSet) Duration() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.end.IsZero() {
		return time.Since(m.start)
	}
	return m.end.Sub(m.start)
}

// StartTime returns the window start.
func (m *MetricSet) StartTime() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.start
}

// OpSnapshot is the computed summary for one operation.
type OpSnapshot struct {
	Name       string
	Latency    LatencyStats
	Errors     map[string]int64
	ErrorTotal int64
	Total      int64
}

// Snapshot is the computed summary for a whole run.
type Snapshot struct {
	Ops        []OpSnapshot
	Duration   time.Duration
	Start      time.Time
	TotalOps   int64
	TotalError int64
}

// Snapshot computes summaries for every recorded operation, sorted by name.
func (m *MetricSet) Snapshot() Snapshot {
	m.mu.Lock()
	names := make([]string, 0, len(m.ops))
	for k := range m.ops {
		names = append(names, k)
	}
	ops := m.ops
	m.mu.Unlock()
	sort.Strings(names)

	snap := Snapshot{Duration: m.Duration(), Start: m.StartTime()}
	for _, name := range names {
		op := ops[name]
		ls := op.Latency.Snapshot()
		failures := op.Errors.Failures()
		snap.Ops = append(snap.Ops, OpSnapshot{
			Name:       name,
			Latency:    ls,
			Errors:     op.Errors.Snapshot(),
			ErrorTotal: failures,
			Total:      ls.Count,
		})
		snap.TotalOps += ls.Count
		snap.TotalError += failures
	}
	return snap
}
