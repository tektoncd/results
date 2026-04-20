// Copyright 2026 The Tekton Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logs

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

func TestBuildLogListFilter(t *testing.T) {
	const wantBase = `(data_type == "results.tekton.dev/v1alpha3.Log" || data_type == "results.tekton.dev/v1alpha2.Log")`
	for _, tc := range []struct {
		user string
		want string
	}{
		{"", wantBase},
		{`data.spec.resource.kind=TaskRun`, `(data.spec.resource.kind=TaskRun) && ` + wantBase},
	} {
		got := buildLogListFilter(tc.user)
		if diff := cmp.Diff(tc.want, got); diff != "" {
			t.Errorf("buildLogListFilter(%q) diff (-want +got):\n%s", tc.user, diff)
		}
	}
}

func TestRewriteLogRecordNames(t *testing.T) {
	tests := []struct {
		name    string
		records []*pb.Record
		want    []string
	}{
		{
			name:    "single valid record",
			records: []*pb.Record{{Name: "ns/results/res-1/records/rec-log"}},
			want:    []string{"ns/results/res-1/logs/rec-log"},
		},
		{
			name: "multiple valid records",
			records: []*pb.Record{
				{Name: "ns/results/res-1/records/rec-log-1"},
				{Name: "ns/results/res-2/records/rec-log-2"},
				{Name: "ns/results/res-3/records/rec-log-3"},
			},
			want: []string{
				"ns/results/res-1/logs/rec-log-1",
				"ns/results/res-2/logs/rec-log-2",
				"ns/results/res-3/logs/rec-log-3",
			},
		},
		{
			name: "mix of valid and invalid names",
			records: []*pb.Record{
				{Name: "ns/results/res-1/records/rec-log-1"},
				{Name: "invalid-name"},
				{Name: "ns/results/res-2/records/rec-log-2"},
				{Name: ""},
				{Name: "ns/results/res-3/records/rec-log-3"},
			},
			want: []string{
				"ns/results/res-1/logs/rec-log-1",
				"invalid-name", // unchanged
				"ns/results/res-2/logs/rec-log-2",
				"", // unchanged
				"ns/results/res-3/logs/rec-log-3",
			},
		},
		{
			name:    "nil record in list",
			records: []*pb.Record{{Name: "ns/results/res-1/records/rec-log"}, nil, {Name: "ns/results/res-2/records/rec-log-2"}},
			want:    []string{"ns/results/res-1/logs/rec-log", "", "ns/results/res-2/logs/rec-log-2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := rewriteLogRecordNames(tt.records); err != nil {
				t.Fatalf("rewriteLogRecordNames returned error: %v", err)
			}

			for i, record := range tt.records {
				if record == nil {
					continue
				}
				if record.Name != tt.want[i] {
					t.Errorf("record[%d].Name = %q, want %q", i, record.Name, tt.want[i])
				}
			}
		})
	}
}
