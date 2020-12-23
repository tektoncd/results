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

package pagination

import (
	"strconv"
	"testing"
)

func TestEncodeDecodeToken(t *testing.T) {
	name := "foo"
	filter := "bar"
	token := "Q2dObWIyOFNBMkpoY2c"

	gotToken, err := EncodeToken(name, filter)
	if err != nil {
		t.Fatalf("EncodeToken: %v", err)
	}
	if token != gotToken {
		t.Errorf("EncodeToken want: %s, got %s", token, gotToken)
	}

	gotName, gotFilter, err := DecodeToken(gotToken)
	if err != nil {
		t.Fatalf("DecodeToken: %v", err)
	}
	if (name != gotName) || (filter != gotFilter) {
		t.Errorf("EncodeToken want: (%s, %s), got (%s, %s)", name, gotName, filter, gotFilter)
	}
}

type batchSequence struct {
	want    int // what number we expect from this call to Next()
	fetched int // simulated number of returned results to feed into Update().
}

func TestBatcher(t *testing.T) {
	for _, tc := range []struct {
		pageSize int
		seq      []batchSequence
	}{
		{
			pageSize: 100,
			seq: []batchSequence{
				{want: 100, fetched: 50},
				{want: 200, fetched: 10},
				{want: 1000, fetched: 10},
				{want: 1000}, // Caps at max
			},
		},
		{
			pageSize: 100,
			seq: []batchSequence{
				{want: 100, fetched: 80},
				{want: 125, fetched: 20},
				{want: 625},
			},
		},
		{
			pageSize: 1,
			seq: []batchSequence{
				{want: 1, fetched: 5},
				{want: 1, fetched: 1},
				{want: 1},
			},
		},
	} {
		t.Run(strconv.Itoa(tc.pageSize), func(t *testing.T) {
			b := NewBatcher(tc.pageSize, 10, 1000)
			for i, tc := range tc.seq {
				got := b.Next()
				if got != tc.want {
					t.Errorf("step (%d) - want: %d, got %d", i, tc.want, got)
				}
				b.Update(tc.fetched, got)
			}
		})
	}
}
