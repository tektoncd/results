// Copyright 2020 The Tekton Authors
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

package server

import (
	"fmt"
	"testing"
)

func TestPageSize(t *testing.T) {
	for _, tc := range []struct {
		in   int
		want int
		err  bool
	}{
		{
			in:   1,
			want: 1,
		},
		{
			in:  -1,
			err: true,
		},
		{
			in:   int(^uint32(0) >> 1), // Max int32
			want: maxPageSize,
		},
	} {
		t.Run(fmt.Sprintf("%d", tc.in), func(t *testing.T) {
			got, err := pageSize(tc.in)
			if got != tc.want || (err == nil && tc.err) {
				t.Errorf("want (%d, %t), got (%d, %v)", tc.want, tc.err, got, err)
			}
		})
	}
}

func TestPageStart(t *testing.T) {
	for _, tc := range []struct {
		name   string
		token  string
		filter string
		want   string
		err    bool
	}{
		{
			name:   "success",
			token:  pagetoken(t, "a", "b"),
			filter: "b",
			want:   "a",
		},
		{
			name:  "wrong filter",
			token: pagetoken(t, "a", "c"),
			err:   true,
		},
		{
			name:  "invalid token",
			token: "tacocat",
			err:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pageStart(tc.token, tc.filter)
			if got != tc.want || (err == nil && tc.err) {
				t.Errorf("want (%s, %t), got (%s, %v)", tc.want, tc.err, got, err)
			}
		})
	}
}
