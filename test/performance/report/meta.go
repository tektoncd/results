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
	"context"
	"os"
	"os/exec"
	"strings"
)

// CaptureMeta fills in the environment-derived provenance fields (git commit,
// dirty state, hostname). Callers supply the run-specific fields (tier, mode,
// dataset, backend, server image) on the returned struct. Failures to read git
// or the hostname are non-fatal — the corresponding field is left empty.
func CaptureMeta(ctx context.Context) Meta {
	meta := Meta{}
	if commit, err := gitOutput(ctx, "rev-parse", "HEAD"); err == nil {
		meta.GitCommit = commit
		// Only meaningful inside a git repo: a non-zero exit from
		// `git diff --quiet` means the working tree has uncommitted changes.
		if _, err := gitOutput(ctx, "diff", "--quiet"); err != nil {
			meta.GitDirty = true
		}
	}
	if host, err := os.Hostname(); err == nil {
		meta.Hostname = host
	}
	return meta
}

// gitOutput runs a git command and returns its trimmed stdout.
func gitOutput(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", args...).Output() //nolint:gosec // fixed "git" binary, caller-controlled static args
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
