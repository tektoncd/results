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
	"os"
	"path/filepath"
	"testing"
)

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

func TestWithOutputEmptyPathUsesStdout(t *testing.T) {
	// An empty path is the default and must stream to stdout without creating any
	// file, preserving the JSON-to-stdout contract for the dataset subcommand.
	var sink *os.File
	if err := withOutput("", func(w *os.File) error {
		sink = w
		return nil
	}); err != nil {
		t.Fatalf("withOutput(\"\") = %v, want nil", err)
	}
	if sink != os.Stdout {
		t.Errorf("withOutput(\"\") sink = %v, want os.Stdout", sink)
	}
}
