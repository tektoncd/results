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
