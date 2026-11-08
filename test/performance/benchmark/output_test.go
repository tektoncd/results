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
	"path/filepath"
	"regexp"
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

func TestEmitReportWritesFileAndStdout(t *testing.T) {
	for _, mode := range []string{"load", "store", "query", "mixed"} {
		for _, output := range []string{"explicit path", "default path", "default without git"} {
			t.Run(mode+"/"+output, func(t *testing.T) {
				defaultDir := t.TempDir()
				t.Setenv("TMPDIR", defaultDir)
				cfg := testRunConfig()
				if output == "explicit path" {
					cfg.OutputPath = filepath.Join(t.TempDir(), "nested", "report with spaces.json")
				}
				if output == "default without git" {
					t.Setenv("PATH", t.TempDir())
				}
				stdout, err := captureStdout(t, func() error {
					if mode == "load" || mode == "store" {
						if err := writeSendLog(cfg.OutputPath, &sendLog{}); err != nil {
							return err
						}
					}
					return emitReport(context.Background(), cfg, mode, "grpc", reportExtra{}, metrics.Snapshot{TotalOps: 3, TotalError: 1})
				})
				if err != nil {
					t.Fatalf("emitReport() = %v", err)
				}
				path := cfg.OutputPath
				if path == "" {
					files, err := os.ReadDir(defaultDir)
					if err != nil {
						t.Fatal(err)
					}
					if len(files) != 1 {
						t.Fatalf("default output directory has %d entries, want one report", len(files))
					}
					path = filepath.Join(defaultDir, files[0].Name())
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if string(stdout) != string(data) {
					t.Error("stdout does not match the saved report")
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
}

func TestEmitReportStdoutFailurePreservesFile(t *testing.T) {
	cfg := testRunConfig()
	cfg.OutputPath = filepath.Join(t.TempDir(), "report.json")
	_, err := captureStdout(t, func() error {
		if err := os.Stdout.Close(); err != nil {
			return err
		}
		return emitReport(context.Background(), cfg, "store", "grpc", reportExtra{}, metrics.Snapshot{})
	})
	if !errors.Is(err, os.ErrClosed) {
		t.Fatalf("emitReport() = %v, want closed stdout error", err)
	}
	data, err := os.ReadFile(cfg.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(data) {
		t.Fatal("stdout failure did not preserve a valid report file")
	}
}

func TestDatasetCommandOutputFile(t *testing.T) {
	defaultDir := t.TempDir()
	t.Setenv("TMPDIR", defaultDir)
	path := filepath.Join(t.TempDir(), "nested", "dataset with spaces.json")
	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"dataset", "--count", "2", "--child-min", "2", "--child-max", "2", "--output", path,
	})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("dataset --output: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading dataset at requested output path: %v", err)
	}
	var got generator.DatasetDefinition
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decoding dataset: %v", err)
	}
	if got.Count != 2 || len(got.Instances) != 2 || got.ContentHash == "" {
		t.Errorf("dataset count/instances/hash = %d/%d/%q, want 2/2/nonempty", got.Count, len(got.Instances), got.ContentHash)
	}
	files, err := os.ReadDir(defaultDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Errorf("explicit --output left %d entries in the default report directory", len(files))
	}
}

func TestWithOutputCreatesParentDirs(t *testing.T) {
	// A nested path whose parent directories do not yet exist must be created on
	// demand, mirroring an operator pointing --output at test/performance/output/.
	path := filepath.Join(t.TempDir(), "a", "b", "c", "store.json")
	if err := withOutput(path, func(w *os.File) error {
		_, err := w.WriteString("{}\n")
		return err
	}); err != nil {
		t.Fatalf("withOutput(%q) = %v, want nil", path, err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading report: %v", err)
	}
	if want := "{}\n"; string(got) != want {
		t.Errorf("report contents = %q, want %q", got, want)
	}
}

func TestWithOutputEmptyPathUsesTemporaryFile(t *testing.T) {
	for _, tt := range []struct {
		name       string
		withoutGit bool
		pattern    string
	}{
		{name: "default environment", pattern: `^\d{8}-\d{6}(-[0-9a-f]+)?\.json$`},
		{name: "git unavailable", withoutGit: true, pattern: `^\d{8}-\d{6}\.json$`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("TMPDIR", dir)
			if tt.withoutGit {
				t.Setenv("PATH", t.TempDir())
			}
			var path string
			if err := withOutput("", func(w *os.File) error {
				path = w.Name()
				_, err := w.WriteString("{}\n")
				return err
			}); err != nil {
				t.Fatalf("withOutput(\"\") = %v, want nil", err)
			}
			if filepath.Dir(path) != dir {
				t.Fatalf("report path = %q, want a file under %q", path, dir)
			}
			if !regexp.MustCompile(tt.pattern).MatchString(filepath.Base(path)) {
				t.Errorf("report filename = %q, want pattern %q", filepath.Base(path), tt.pattern)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading report: %v", err)
			}
			if want := "{}\n"; string(got) != want {
				t.Errorf("report contents = %q, want %q", got, want)
			}
		})
	}
}
