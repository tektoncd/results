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
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/tektoncd/results/test/performance/report"
)

// defaultThreshold is the relative change that flags a regression: ±10% on p99
// latency and throughput, per the tier-1 guideline. The tier-2 threshold is
// documented as TBD until the first ephemeral-cluster runs establish variance.
const defaultThreshold = 0.10

// metricDelta is the change in one scalar metric from baseline to candidate.
// PctChange is the raw fractional change (candidate-baseline)/baseline; its sign
// is literal, not "better/worse" — Regression already encodes the direction.
type metricDelta struct {
	Name           string
	Baseline       float64
	Candidate      float64
	PctChange      float64
	HigherIsBetter bool
	Regression     bool
}

// comparison is the full result of comparing two reports.
type comparison struct {
	Warnings []string
	Deltas   []metricDelta
}

// regressions returns the deltas that breached the threshold.
func (c comparison) regressions() []metricDelta {
	var out []metricDelta
	for _, d := range c.Deltas {
		if d.Regression {
			out = append(out, d)
		}
	}
	return out
}

// compareReports diffs candidate against baseline and flags metrics whose change
// exceeds threshold in the unfavorable direction. Throughput is higher-is-better;
// per-op p99 latency is lower-is-better. Mismatched mode or dataset hash yields a
// warning (not a failure) because the numbers are then not strictly comparable.
func compareReports(baseline, candidate *report.Report, threshold float64) comparison {
	var c comparison
	if baseline.Meta.Mode != candidate.Meta.Mode {
		c.Warnings = append(c.Warnings, fmt.Sprintf("mode differs: baseline %q vs candidate %q", baseline.Meta.Mode, candidate.Meta.Mode))
	}
	if baseline.Meta.DatasetHash != candidate.Meta.DatasetHash {
		c.Warnings = append(c.Warnings, fmt.Sprintf("dataset hash differs: baseline %q vs candidate %q (results may not be comparable)", baseline.Meta.DatasetHash, candidate.Meta.DatasetHash))
	}

	c.Deltas = append(c.Deltas, newDelta("throughput_per_sec", baseline.Metrics.ThroughputPerSec, candidate.Metrics.ThroughputPerSec, threshold, true))

	for _, op := range sortedOps(baseline.Metrics.ByOp, candidate.Metrics.ByOp) {
		b, inBase := baseline.Metrics.ByOp[op]
		cand, inCand := candidate.Metrics.ByOp[op]
		if !inBase {
			c.Warnings = append(c.Warnings, fmt.Sprintf("operation %q present in candidate but not baseline", op))
			continue
		}
		if !inCand {
			c.Warnings = append(c.Warnings, fmt.Sprintf("operation %q present in baseline but not candidate", op))
			continue
		}
		c.Deltas = append(c.Deltas, newDelta(op+".p99_ms", b.P99MS, cand.P99MS, threshold, false))
	}
	return c
}

// newDelta computes one metric delta and decides whether it is a regression.
func newDelta(name string, baseline, candidate, threshold float64, higherIsBetter bool) metricDelta {
	d := metricDelta{Name: name, Baseline: baseline, Candidate: candidate, HigherIsBetter: higherIsBetter}
	if baseline == 0 {
		// No baseline to measure against; report the raw values without flagging.
		return d
	}
	d.PctChange = (candidate - baseline) / baseline
	if higherIsBetter {
		d.Regression = d.PctChange < -threshold
	} else {
		d.Regression = d.PctChange > threshold
	}
	return d
}

// sortedOps returns the union of operation names across both reports, sorted so
// output ordering is deterministic.
func sortedOps(a, b map[string]report.OpReport) []string {
	seen := make(map[string]bool, len(a)+len(b))
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	ops := make([]string, 0, len(seen))
	for k := range seen {
		ops = append(ops, k)
	}
	sort.Strings(ops)
	return ops
}

// writeComparison renders the comparison as a human-readable table.
func writeComparison(w io.Writer, c comparison) {
	for _, warn := range c.Warnings {
		_, _ = fmt.Fprintf(w, "warning: %s\n", warn)
	}
	_, _ = fmt.Fprintf(w, "%-40s %14s %14s %10s  %s\n", "METRIC", "BASELINE", "CANDIDATE", "CHANGE", "VERDICT")
	for _, d := range c.Deltas {
		_, _ = fmt.Fprintf(w, "%-40s %14.2f %14.2f %+9.1f%%  %s\n", d.Name, d.Baseline, d.Candidate, d.PctChange*100, verdict(d))
	}
}

// verdict labels a delta for the human-readable table.
func verdict(d metricDelta) string {
	switch {
	case d.Regression:
		return "REGRESSION"
	case d.Baseline == 0:
		return "n/a"
	case d.HigherIsBetter && d.PctChange > 0, !d.HigherIsBetter && d.PctChange < 0:
		return "improved"
	default:
		return "ok"
	}
}

// readReport reads and decodes a report from path.
func readReport(path string) (*report.Report, error) {
	f, err := os.Open(path) //nolint:gosec // operator-provided report path
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	rep, err := report.Read(f)
	if err != nil {
		return nil, fmt.Errorf("decoding report %q: %w", path, err)
	}
	return rep, nil
}
