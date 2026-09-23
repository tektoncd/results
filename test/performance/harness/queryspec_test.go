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
	"strings"
	"testing"

	"github.com/tektoncd/results/test/performance/harness/workload"
)

func TestPlanFromSpecExpandsByWeight(t *testing.T) {
	spec := &workload.QueryMixSpec{
		SchemaVersion: workload.SchemaVersion,
		Operations: []workload.OperationWeight{
			{Type: workload.OpLabelSelectorList, Kind: "pipelineruns", Weight: 0.75},
			{Type: workload.OpListByNamespaceKind, Kind: "taskruns", Weight: 0.25},
		},
		PageSizes: []workload.PageSizeWeight{
			{Limit: 30, Weight: 0.8},
			{Limit: 500, Weight: 0.2},
		},
		LabelSelector: workload.LabelSelectorMix{
			Keys: []workload.LabelKeyWeight{{Key: "appstudio.openshift.io/application", Weight: 1}},
		},
	}

	plan, err := planFromSpec(spec)
	if err != nil {
		t.Fatalf("planFromSpec() error: %v", err)
	}

	labelCount, listCount := 0, 0
	for _, q := range plan.queries {
		switch q.Op {
		case "list_records_label":
			labelCount++
		case "list_records_type":
			listCount++
		default:
			t.Errorf("unexpected op %q", q.Op)
		}
	}
	// 0.75 vs 0.25 at resolution 1000 → exactly 750 / 250.
	if labelCount != 750 || listCount != 250 {
		t.Errorf("query expansion = label %d, list %d; want 750, 250", labelCount, listCount)
	}

	p30, p500 := 0, 0
	for _, ps := range plan.pageSizes {
		switch ps {
		case 30:
			p30++
		case 500:
			p500++
		default:
			t.Errorf("unexpected page size %d", ps)
		}
	}
	if p30 != 800 || p500 != 200 {
		t.Errorf("page-size expansion = 30:%d, 500:%d; want 800, 200", p30, p500)
	}
}

func TestQueryForOp(t *testing.T) {
	tests := []struct {
		name       string
		op         workload.OperationWeight
		labelKey   string
		wantOp     string
		wantSingle bool
		wantFilter string // substring that must be present
	}{
		{
			name:       "label selector list uses top key and data type",
			op:         workload.OperationWeight{Type: workload.OpLabelSelectorList, Kind: "pipelineruns"},
			labelKey:   "appstudio.openshift.io/application",
			wantOp:     "list_records_label",
			wantFilter: `data.metadata.labels["appstudio.openshift.io/application"]`,
		},
		{
			name:       "get by name is a single-page point lookup",
			op:         workload.OperationWeight{Type: workload.OpGetByName, Kind: "pipelineruns"},
			wantOp:     "get_record_by_name",
			wantSingle: true,
			wantFilter: "data_type == PIPELINE_RUN",
		},
		{
			name:       "owner uid lookup is a single-page point lookup",
			op:         workload.OperationWeight{Type: workload.OpOwnerUIDLookup, Kind: "taskruns"},
			wantOp:     "owner_uid_lookup",
			wantSingle: true,
			wantFilter: "data_type == TASK_RUN",
		},
		{
			name:       "plain list by kind",
			op:         workload.OperationWeight{Type: workload.OpListByNamespaceKind, Kind: "taskruns"},
			wantOp:     "list_records_type",
			wantFilter: "data_type == TASK_RUN",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := queryForOp(tc.op, tc.labelKey)
			if q.Op != tc.wantOp {
				t.Errorf("Op = %q, want %q", q.Op, tc.wantOp)
			}
			if q.Single != tc.wantSingle {
				t.Errorf("Single = %v, want %v", q.Single, tc.wantSingle)
			}
			if !strings.Contains(q.Filter, tc.wantFilter) {
				t.Errorf("Filter = %q, want substring %q", q.Filter, tc.wantFilter)
			}
		})
	}
}

func TestNamespaceSequenceHonorsTopShare(t *testing.T) {
	seq := namespaceSequence(10, workload.NamespaceFanout{DistinctCount: 5, TopShare: 0.7})
	if len(seq) == 0 {
		t.Fatal("empty namespace sequence")
	}
	hot := 0
	for _, idx := range seq {
		if idx < 0 || idx >= 10 {
			t.Fatalf("namespace index %d out of range [0,10)", idx)
		}
		if idx == 0 {
			hot++
		}
	}
	share := float64(hot) / float64(len(seq))
	if share < 0.65 || share > 0.75 {
		t.Errorf("hot-namespace share = %.2f, want ~0.70", share)
	}
}

func TestNamespaceSequenceEdgeCases(t *testing.T) {
	if got := namespaceSequence(1, workload.NamespaceFanout{TopShare: 0.9}); len(got) != 1 || got[0] != 0 {
		t.Errorf("single namespace = %v, want [0]", got)
	}
	// TopShare of 0 falls back to an even spread over the whole pool.
	got := namespaceSequence(4, workload.NamespaceFanout{TopShare: 0})
	if len(got) != 4 {
		t.Errorf("even spread length = %d, want 4", len(got))
	}
}

func TestBuildQueryScheduleLegacy(t *testing.T) {
	queries, pageSizes, nsSeq, err := buildQuerySchedule("", true, 250, 8)
	if err != nil {
		t.Fatalf("buildQuerySchedule() error: %v", err)
	}
	if len(queries) != len(defaultQueries()) {
		t.Errorf("legacy queries = %d, want %d", len(queries), len(defaultQueries()))
	}
	if len(pageSizes) != 1 || pageSizes[0] != 250 {
		t.Errorf("legacy page sizes = %v, want [250]", pageSizes)
	}
	if len(nsSeq) != 8 {
		t.Errorf("legacy namespace sequence length = %d, want 8", len(nsSeq))
	}
}

func TestBuildQueryScheduleFromEmbeddedSpec(t *testing.T) {
	queries, pageSizes, nsSeq, err := buildQuerySchedule("", false, 1000, 8)
	if err != nil {
		t.Fatalf("buildQuerySchedule() error: %v", err)
	}
	if len(queries) == 0 || len(pageSizes) == 0 || len(nsSeq) == 0 {
		t.Errorf("empty schedule from embedded spec: queries=%d pageSizes=%d nsSeq=%d", len(queries), len(pageSizes), len(nsSeq))
	}
}

func TestExpandCount(t *testing.T) {
	tests := []struct {
		weight float64
		want   int
	}{
		{0.75, 750},
		{0.001, 1}, // rounds to 1 (never drops an observed entry)
		{0, 1},     // minimum one slot
		{1, 1000},
	}
	for _, tc := range tests {
		if got := expandCount(tc.weight); got != tc.want {
			t.Errorf("expandCount(%v) = %d, want %d", tc.weight, got, tc.want)
		}
	}
}
