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

package convert

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseFieldPaths(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		want    [][]string
		wantErr bool
	}{
		{raw: "", want: nil},
		{raw: " , ", want: nil},
		{raw: "metadata.managedFields", want: [][]string{{"metadata", "managedFields"}}},
		{raw: " metadata.managedFields , status.provenance ,", want: [][]string{{"metadata", "managedFields"}, {"status", "provenance"}}},
		{raw: "metadata..managedFields", wantErr: true},
		{raw: ".status", wantErr: true},
		{raw: "kind", wantErr: true},
		{raw: "metadata", wantErr: true},
		{raw: "metadata.name", wantErr: true},
		{raw: "metadata.uid", wantErr: true},
		{raw: "metadata.labels", want: [][]string{{"metadata", "labels"}}},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := ParseFieldPaths(tc.raw)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseFieldPaths(%q) error = %v, wantErr %v", tc.raw, err, tc.wantErr)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("ParseFieldPaths(%q) (-want, +got):\n%s", tc.raw, diff)
			}
		})
	}
}

func TestStripFields(t *testing.T) {
	in := `{
		"apiVersion": "tekton.dev/v1",
		"kind": "TaskRun",
		"metadata": {
			"name": "taskrun",
			"namespace": "test",
			"generation": 9007199254740993,
			"managedFields": [{"manager": "controller"}]
		},
		"status": {
			"provenance": {"featureFlags": {}},
			"steps": [{"name": "build"}]
		}
	}`
	paths, err := ParseFieldPaths("metadata.managedFields,status.provenance,status.steps.name,spec.missing")
	if err != nil {
		t.Fatal(err)
	}
	out, err := StripFields([]byte(in), paths)
	if err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"apiVersion": "tekton.dev/v1",
		"kind":       "TaskRun",
		"metadata": map[string]any{
			"name":       "taskrun",
			"namespace":  "test",
			"generation": 9007199254740993.0,
		},
		"status": map[string]any{
			// Paths are not followed through arrays.
			"steps": []any{map[string]any{"name": "build"}},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("StripFields (-want, +got):\n%s", diff)
	}
	// Large integers must keep their exact value.
	if !strings.Contains(string(out), `"generation":9007199254740993`) {
		t.Errorf("StripFields changed the generation value: %s", out)
	}
}

func TestStripFieldsNoPaths(t *testing.T) {
	in := []byte(`{"kind":"TaskRun"}`)
	out, err := StripFields(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(in) {
		t.Errorf("StripFields with no paths = %s, want the input unchanged", out)
	}
}

func TestStripFieldsInvalidJSON(t *testing.T) {
	paths := [][]string{{"metadata", "managedFields"}}
	if _, err := StripFields([]byte(`[1, 2]`), paths); err == nil {
		t.Error("StripFields on a JSON array: want error")
	}
}
