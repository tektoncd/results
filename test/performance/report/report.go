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

// Package report defines the machine-readable benchmark report schema emitted by
// the harness and consumed by the compare tool (Story 02). It converts a
// metrics.Snapshot into a stable, versioned JSON document annotated with run
// metadata (git commit, dataset version, tier, backend).
package report

import (
	"encoding/json"
	"io"
	"time"

	"github.com/tektoncd/results/test/performance/metrics"
)

// SchemaVersion identifies the report format; bump on any breaking field change.
const SchemaVersion = "1"

// Report is the top-level benchmark artifact.
type Report struct {
	SchemaVersion string  `json:"schema_version"`
	Meta          Meta    `json:"meta"`
	Config        Config  `json:"config"`
	Metrics       Metrics `json:"metrics"`
}

// Meta captures the provenance needed to compare two runs meaningfully.
type Meta struct {
	GitCommit      string `json:"git_commit"`
	GitDirty       bool   `json:"git_dirty"`
	DatasetVersion string `json:"dataset_version"`
	DatasetHash    string `json:"dataset_hash"`
	Tier           string `json:"tier"`
	Mode           string `json:"mode"`
	StartedAt      string `json:"started_at"`
	DurationMS     int64  `json:"duration_ms"`
	Hostname       string `json:"hostname"`
	APIServerAddr  string `json:"api_server_addr"`
	DBBackend      string `json:"db_backend"`
	ServerImage    string `json:"server_image"`
}

// Config records the knobs that shaped the run so results are reproducible.
type Config struct {
	Count       int    `json:"count"`
	Concurrency int    `json:"concurrency"`
	DurationSec int    `json:"duration_sec"`
	Transport   string `json:"transport"`
	ReadRatio   int    `json:"read_ratio"`
	WriteRatio  int    `json:"write_ratio"`
	Dataset     string `json:"dataset"`
	Seed        int64  `json:"seed"`
}

// Metrics is the aggregate performance result.
type Metrics struct {
	ThroughputPerSec float64             `json:"throughput_per_sec"`
	TotalOps         int64               `json:"total_ops"`
	TotalErrors      int64               `json:"total_errors"`
	ByOp             map[string]OpReport `json:"by_op"`
}

// OpReport is the per-operation summary in report units (milliseconds).
type OpReport struct {
	Count      int64            `json:"count"`
	Errors     int64            `json:"errors"`
	ErrorCodes map[string]int64 `json:"error_codes"`
	P50MS      float64          `json:"p50_ms"`
	P90MS      float64          `json:"p90_ms"`
	P99MS      float64          `json:"p99_ms"`
	MinMS      float64          `json:"min_ms"`
	MaxMS      float64          `json:"max_ms"`
	MeanMS     float64          `json:"mean_ms"`
}

// New assembles a Report from run metadata, config, and a metrics snapshot.
func New(meta Meta, cfg Config, snap metrics.Snapshot) *Report {
	meta.DurationMS = snap.Duration.Milliseconds()
	if meta.StartedAt == "" && !snap.Start.IsZero() {
		meta.StartedAt = snap.Start.UTC().Format(time.RFC3339)
	}
	return &Report{
		SchemaVersion: SchemaVersion,
		Meta:          meta,
		Config:        cfg,
		Metrics:       buildMetrics(snap),
	}
}

// buildMetrics converts the metrics snapshot into report units.
func buildMetrics(snap metrics.Snapshot) Metrics {
	m := Metrics{
		TotalOps:    snap.TotalOps,
		TotalErrors: snap.TotalError,
		ByOp:        make(map[string]OpReport, len(snap.Ops)),
	}
	if secs := snap.Duration.Seconds(); secs > 0 {
		m.ThroughputPerSec = float64(snap.TotalOps) / secs
	}
	for _, op := range snap.Ops {
		m.ByOp[op.Name] = OpReport{
			Count:      op.Total,
			Errors:     op.ErrorTotal,
			ErrorCodes: op.Errors,
			P50MS:      ms(op.Latency.P50),
			P90MS:      ms(op.Latency.P90),
			P99MS:      ms(op.Latency.P99),
			MinMS:      ms(op.Latency.Min),
			MaxMS:      ms(op.Latency.Max),
			MeanMS:     ms(op.Latency.Mean),
		}
	}
	return m
}

// ms converts a duration to fractional milliseconds.
func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

// Write emits the report as indented JSON with a trailing newline.
func Write(w io.Writer, r *Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// Read decodes a report from JSON.
func Read(r io.Reader) (*Report, error) {
	rep := &Report{}
	if err := json.NewDecoder(r).Decode(rep); err != nil {
		return nil, err
	}
	return rep, nil
}
