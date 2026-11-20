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
	"strings"
	"testing"

	"github.com/tektoncd/results/test/performance/report"
)

// mkReport builds a minimal report for comparison tests.
func mkReport(mode, hash string, throughput float64, p99 map[string]float64) *report.Report {
	byOp := make(map[string]report.OpReport, len(p99))
	for k, v := range p99 {
		byOp[k] = report.OpReport{P99MS: v}
	}
	return &report.Report{
		Meta:    report.Meta{Mode: mode, DatasetHash: hash},
		Metrics: report.Metrics{ThroughputPerSec: throughput, ByOp: byOp},
	}
}

func TestCompareReportsRegressions(t *testing.T) {
	tests := []struct {
		name           string
		baseline       *report.Report
		candidate      *report.Report
		threshold      float64
		wantRegression bool
	}{
		{
			name:           "identical",
			baseline:       mkReport("query", "h1", 100, map[string]float64{"list": 50}),
			candidate:      mkReport("query", "h1", 100, map[string]float64{"list": 50}),
			threshold:      0.10,
			wantRegression: false,
		},
		{
			name:           "latency regression beyond threshold",
			baseline:       mkReport("query", "h1", 100, map[string]float64{"list": 50}),
			candidate:      mkReport("query", "h1", 100, map[string]float64{"list": 60}), // +20%
			threshold:      0.10,
			wantRegression: true,
		},
		{
			name:           "latency regression within threshold is ok",
			baseline:       mkReport("query", "h1", 100, map[string]float64{"list": 50}),
			candidate:      mkReport("query", "h1", 100, map[string]float64{"list": 54}), // +8%
			threshold:      0.10,
			wantRegression: false,
		},
		{
			name:           "latency improvement is not a regression",
			baseline:       mkReport("query", "h1", 100, map[string]float64{"list": 50}),
			candidate:      mkReport("query", "h1", 100, map[string]float64{"list": 40}), // -20%
			threshold:      0.10,
			wantRegression: false,
		},
		{
			name:           "throughput regression beyond threshold",
			baseline:       mkReport("query", "h1", 100, map[string]float64{"list": 50}),
			candidate:      mkReport("query", "h1", 80, map[string]float64{"list": 50}), // -20%
			threshold:      0.10,
			wantRegression: true,
		},
		{
			name:           "throughput improvement is not a regression",
			baseline:       mkReport("query", "h1", 100, map[string]float64{"list": 50}),
			candidate:      mkReport("query", "h1", 130, map[string]float64{"list": 50}), // +30%
			threshold:      0.10,
			wantRegression: false,
		},
		{
			name:           "zero baseline latency is not flagged",
			baseline:       mkReport("query", "h1", 100, map[string]float64{"list": 0}),
			candidate:      mkReport("query", "h1", 100, map[string]float64{"list": 99}),
			threshold:      0.10,
			wantRegression: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := compareReports(tc.baseline, tc.candidate, tc.threshold)
			got := len(c.regressions()) > 0
			if got != tc.wantRegression {
				t.Errorf("compareReports() regression = %v, want %v (deltas: %+v)", got, tc.wantRegression, c.Deltas)
			}
		})
	}
}

func TestCompareReportsWarnings(t *testing.T) {
	tests := []struct {
		name        string
		baseline    *report.Report
		candidate   *report.Report
		wantWarning string
	}{
		{
			name:        "mode mismatch",
			baseline:    mkReport("query", "h1", 100, map[string]float64{"list": 50}),
			candidate:   mkReport("mixed", "h1", 100, map[string]float64{"list": 50}),
			wantWarning: "mode differs",
		},
		{
			name:        "dataset hash mismatch",
			baseline:    mkReport("query", "h1", 100, map[string]float64{"list": 50}),
			candidate:   mkReport("query", "h2", 100, map[string]float64{"list": 50}),
			wantWarning: "dataset hash differs",
		},
		{
			name:        "op only in candidate",
			baseline:    mkReport("query", "h1", 100, map[string]float64{"list": 50}),
			candidate:   mkReport("query", "h1", 100, map[string]float64{"list": 50, "get": 5}),
			wantWarning: `operation "get" present in candidate but not baseline`,
		},
		{
			name:        "op only in baseline",
			baseline:    mkReport("query", "h1", 100, map[string]float64{"list": 50, "get": 5}),
			candidate:   mkReport("query", "h1", 100, map[string]float64{"list": 50}),
			wantWarning: `operation "get" present in baseline but not candidate`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := compareReports(tc.baseline, tc.candidate, 0.10)
			if !containsWarning(c.Warnings, tc.wantWarning) {
				t.Errorf("compareReports() warnings = %v, want one containing %q", c.Warnings, tc.wantWarning)
			}
		})
	}
}

// TestCompareReportsOnlyOpDoesNotRegress guards that a candidate-only op produces a
// warning but never a regression (there is no baseline to measure it against).
func TestCompareReportsOnlyOpDoesNotRegress(t *testing.T) {
	baseline := mkReport("query", "h1", 100, map[string]float64{"list": 50})
	candidate := mkReport("query", "h1", 100, map[string]float64{"list": 50, "get": 9999})
	c := compareReports(baseline, candidate, 0.10)
	if regs := c.regressions(); len(regs) != 0 {
		t.Errorf("candidate-only op flagged as regression: %+v", regs)
	}
}

func TestWriteComparison(t *testing.T) {
	baseline := mkReport("query", "h1", 100, map[string]float64{"list": 50})
	candidate := mkReport("mixed", "h1", 80, map[string]float64{"list": 60})
	c := compareReports(baseline, candidate, 0.10)

	var sb strings.Builder
	writeComparison(&sb, c)
	out := sb.String()
	for _, want := range []string{"warning: mode differs", "throughput_per_sec", "list.p99_ms", "REGRESSION"} {
		if !strings.Contains(out, want) {
			t.Errorf("writeComparison() output missing %q:\n%s", want, out)
		}
	}
}

func containsWarning(warnings []string, substr string) bool {
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}
