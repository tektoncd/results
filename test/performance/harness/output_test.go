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
	"regexp"
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

func TestWithOutputEmptyPathUsesTemporaryFile(t *testing.T) {
	for _, tt := range []struct {
		name       string
		withoutGit bool
		pattern    string
	}{
		{name: "default environment", pattern: `^\d{8}-\d{6}(-[0-9a-f]+)?$`},
		{name: "git unavailable", withoutGit: true, pattern: `^\d{8}-\d{6}$`},
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
