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
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/tektoncd/results/test/performance/generator"
	"github.com/tektoncd/results/test/performance/metrics"
	"github.com/tektoncd/results/test/performance/report"
)

func captureStdout(t *testing.T, fn func() error) ([]byte, error) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = f
	defer func() {
		os.Stdout = previous
		_ = f.Close()
	}()
	err = fn()
	data, readErr := os.ReadFile(f.Name())
	if readErr != nil {
		t.Fatal(readErr)
	}
	return data, err
}

func captureStderr(t *testing.T, fn func() error) ([]byte, error) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stderr
	os.Stderr = f
	defer func() {
		os.Stderr = previous
		_ = f.Close()
	}()
	err = fn()
	data, readErr := os.ReadFile(f.Name())
	if readErr != nil {
		t.Fatal(readErr)
	}
	return data, err
}

func TestEmitReportWritesStdout(t *testing.T) {
	for _, mode := range []string{"load", "store", "query", "mixed"} {
		t.Run(mode, func(t *testing.T) {
			cfg := testRunConfig()
			stdout, err := captureStdout(t, func() error {
				return emitReport(context.Background(), cfg, mode, "grpc", reportExtra{}, metrics.Snapshot{TotalOps: 3, TotalError: 1})
			})
			if err != nil {
				t.Fatalf("emitReport() = %v", err)
			}
			var got report.Report
			if err := json.Unmarshal(stdout, &got); err != nil {
				t.Fatalf("stdout is not a single JSON report: %v", err)
			}
			if got.Meta.Mode != mode || got.Metrics.TotalOps != 3 || got.Metrics.TotalErrors != 1 {
				t.Errorf("stdout report mode/ops/errors = %s/%d/%d, want %s/3/1", got.Meta.Mode, got.Metrics.TotalOps, got.Metrics.TotalErrors, mode)
			}
		})
	}
}

func TestEmitReportStdoutFailure(t *testing.T) {
	cfg := testRunConfig()
	_, err := captureStdout(t, func() error {
		if err := os.Stdout.Close(); err != nil {
			return err
		}
		return emitReport(context.Background(), cfg, "store", "grpc", reportExtra{}, metrics.Snapshot{})
	})
	if !errors.Is(err, os.ErrClosed) {
		t.Fatalf("emitReport() = %v, want closed stdout error", err)
	}
}

func TestDatasetCommandPrintsStdout(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"dataset", "--count", "2", "--child-min", "2", "--child-max", "2"})
	stdout, err := captureStdout(t, func() error { return cmd.ExecuteContext(context.Background()) })
	if err != nil {
		t.Fatalf("dataset: %v", err)
	}
	var got generator.DatasetDefinition
	if err := json.Unmarshal(stdout, &got); err != nil {
		t.Fatalf("decoding dataset: %v", err)
	}
	if got.Count != 2 || len(got.Instances) != 2 || got.ContentHash == "" {
		t.Errorf("dataset count/instances/hash = %d/%d/%q, want 2/2/nonempty", got.Count, len(got.Instances), got.ContentHash)
	}
}

func TestPrintSendLog(t *testing.T) {
	log := &sendLog{}
	log.add(sendLogEntry{Index: 1, UID: "b", Codes: []string{"OK"}})
	log.add(sendLogEntry{Index: 0, UID: "a", Codes: []string{"OK"}})
	stderr, err := captureStderr(t, func() error { return printSendLog(log) })
	if err != nil {
		t.Fatalf("printSendLog() = %v", err)
	}
	var got []sendLogEntry
	if err := json.Unmarshal(stderr, &got); err != nil {
		t.Fatalf("stderr is not a valid send log: %v", err)
	}
	if len(got) != 2 || got[0].Index != 0 || got[1].Index != 1 {
		t.Errorf("send log = %+v, want entries ordered by index", got)
	}
}
