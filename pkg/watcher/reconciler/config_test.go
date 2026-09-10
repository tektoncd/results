// Copyright 2021 The Tekton Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package reconciler

import (
	"testing"
	"time"
)

func TestGetDisableAnnotationUpdate(t *testing.T) {
	for _, tc := range []struct {
		cfg  *Config
		want bool
	}{
		{
			cfg:  &Config{DisableAnnotationUpdate: true},
			want: true,
		},
		{
			cfg:  &Config{DisableAnnotationUpdate: false},
			want: false,
		},
		{
			cfg:  nil,
			want: false,
		},
	} {
		got := tc.cfg.GetDisableAnnotationUpdate()
		if got != tc.want {
			t.Errorf("Config %+v: want %t, got %t", tc.cfg, tc.want, got)
		}
	}
}

func TestAnnotationFlagSet(t *testing.T) {
	for _, tc := range []struct {
		name    string
		inputs  []string
		want    map[string]AnnotationRequirement
		wantErr bool
	}{
		{
			name:   "key=value with arbitrary value",
			inputs: []string{"ci.example.com/status=passed"},
			want:   map[string]AnnotationRequirement{"ci.example.com/status": {ExactMatch: true, Value: "passed"}},
		},
		{
			name:   "value containing equals - split on first only",
			inputs: []string{"key=a=b=c"},
			want:   map[string]AnnotationRequirement{"key": {ExactMatch: true, Value: "a=b=c"}},
		},
		{
			name:   "existence only (no equals)",
			inputs: []string{"hub.example.com/scheduled"},
			want:   map[string]AnnotationRequirement{"hub.example.com/scheduled": {ExactMatch: false}},
		},
		{
			name:   "multiple calls accumulate entries",
			inputs: []string{"a=1", "b"},
			want:   map[string]AnnotationRequirement{"a": {ExactMatch: true, Value: "1"}, "b": {ExactMatch: false}},
		},
		{
			name:   "whitespace trimmed",
			inputs: []string{"  hub.example.com/scheduled=true  "},
			want:   map[string]AnnotationRequirement{"hub.example.com/scheduled": {ExactMatch: true, Value: "true"}},
		},
		{
			name:    "empty string is rejected",
			inputs:  []string{""},
			wantErr: true,
		},
		{
			name:    "empty key is rejected",
			inputs:  []string{"=value"},
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var f AnnotationFlag
			var err error
			for _, input := range tc.inputs {
				if setErr := f.Set(input); setErr != nil {
					err = setErr
				}
			}
			if tc.wantErr {
				if err == nil {
					t.Fatalf("AnnotationFlag.Set(%v) expected error, got nil", tc.inputs)
				}
				return
			}
			if err != nil {
				t.Fatalf("AnnotationFlag.Set(%v) unexpected error: %v", tc.inputs, err)
			}
			if len(f) != len(tc.want) {
				t.Fatalf("AnnotationFlag after Set(%v) = %v (len %d), want %v (len %d)", tc.inputs, f, len(f), tc.want, len(tc.want))
			}
			for k, wantReq := range tc.want {
				if gotReq, ok := f[k]; !ok {
					t.Errorf("AnnotationFlag missing key %q", k)
				} else if gotReq != wantReq {
					t.Errorf("AnnotationFlag[%q] = %+v, want %+v", k, gotReq, wantReq)
				}
			}
		})
	}
}

func TestAreRequiredAnnotationsReady(t *testing.T) {
	for _, tc := range []struct {
		name        string
		cfg         *Config
		annotations map[string]string
		wantReady   bool
		wantMissing string
	}{
		{
			name:        "nil config is always ready",
			cfg:         nil,
			annotations: nil,
			wantReady:   true,
		},
		{
			name:        "empty required map is always ready",
			cfg:         &Config{},
			annotations: nil,
			wantReady:   true,
		},
		{
			name:        "required annotation present and matching",
			cfg:         &Config{RequiredAnnotations: map[string]AnnotationRequirement{"hub.example.com/scheduled": {ExactMatch: true, Value: "true"}}},
			annotations: map[string]string{"hub.example.com/scheduled": "true"},
			wantReady:   true,
		},
		{
			name:        "required annotation missing",
			cfg:         &Config{RequiredAnnotations: map[string]AnnotationRequirement{"hub.example.com/scheduled": {ExactMatch: true, Value: "true"}}},
			annotations: map[string]string{},
			wantReady:   false,
			wantMissing: "hub.example.com/scheduled",
		},
		{
			name:        "required annotation present but wrong value",
			cfg:         &Config{RequiredAnnotations: map[string]AnnotationRequirement{"hub.example.com/scheduled": {ExactMatch: true, Value: "true"}}},
			annotations: map[string]string{"hub.example.com/scheduled": "false"},
			wantReady:   false,
			wantMissing: "hub.example.com/scheduled",
		},
		{
			name:        "existence only - present",
			cfg:         &Config{RequiredAnnotations: map[string]AnnotationRequirement{"hub.example.com/scheduled": {ExactMatch: false}}},
			annotations: map[string]string{"hub.example.com/scheduled": "anything"},
			wantReady:   true,
		},
		{
			name:        "existence only - missing",
			cfg:         &Config{RequiredAnnotations: map[string]AnnotationRequirement{"hub.example.com/scheduled": {ExactMatch: false}}},
			annotations: map[string]string{},
			wantReady:   false,
			wantMissing: "hub.example.com/scheduled",
		},
		{
			name:        "nil annotations map",
			cfg:         &Config{RequiredAnnotations: map[string]AnnotationRequirement{"hub.example.com/scheduled": {ExactMatch: true, Value: "true"}}},
			annotations: nil,
			wantReady:   false,
			wantMissing: "hub.example.com/scheduled",
		},
		{
			name: "multiple required - all satisfied",
			cfg: &Config{RequiredAnnotations: map[string]AnnotationRequirement{
				"hub.example.com/scheduled": {ExactMatch: true, Value: "true"},
				"syncer.example.com/synced": {ExactMatch: true, Value: "true"},
			}},
			annotations: map[string]string{
				"hub.example.com/scheduled": "true",
				"syncer.example.com/synced": "true",
			},
			wantReady: true,
		},
		{
			name: "multiple required - one missing",
			cfg: &Config{RequiredAnnotations: map[string]AnnotationRequirement{
				"hub.example.com/scheduled": {ExactMatch: true, Value: "true"},
				"syncer.example.com/synced": {ExactMatch: true, Value: "true"},
			}},
			annotations: map[string]string{
				"hub.example.com/scheduled": "true",
			},
			wantReady: false,
		},
		{
			name: "mixed true and arbitrary values - all satisfied",
			cfg: &Config{RequiredAnnotations: map[string]AnnotationRequirement{
				"hub.example.com/scheduled": {ExactMatch: false},
				"ci.example.com/status":     {ExactMatch: true, Value: "passed"},
			}},
			annotations: map[string]string{
				"hub.example.com/scheduled": "true",
				"ci.example.com/status":     "passed",
			},
			wantReady: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			missingKey, ready := tc.cfg.AreRequiredAnnotationsReady(tc.annotations)
			if ready != tc.wantReady {
				t.Errorf("AreRequiredAnnotationsReady() ready = %t, want %t", ready, tc.wantReady)
			}
			if tc.wantMissing != "" && missingKey != tc.wantMissing {
				t.Errorf("AreRequiredAnnotationsReady() missingKey = %q, want %q", missingKey, tc.wantMissing)
			}
		})
	}
}

func TestCompletedResourceGracePeriod(t *testing.T) {
	for _, tc := range []struct {
		cfg  *Config
		want time.Duration
	}{
		{
			cfg:  &Config{CompletedResourceGracePeriod: 0},
			want: time.Duration(0),
		},
		{
			cfg:  &Config{CompletedResourceGracePeriod: -1},
			want: time.Duration(-1),
		},
		{
			cfg:  &Config{CompletedResourceGracePeriod: 1},
			want: time.Duration(1),
		},
	} {
		if got := tc.cfg.GetCompletedResourceGracePeriod(); got != tc.want {
			t.Errorf("Config %+v: Duration want %v, got %v", tc.cfg, got, tc.want)
		}
	}
}
