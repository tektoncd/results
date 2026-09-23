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
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/tektoncd/results/test/performance/harness/workload"
)

// planResolution is the number of slots a weighted distribution is expanded into.
// A weight of w contributes round(w*planResolution) copies (at least one), so the
// query, page-size, and namespace sequences reproduce the spec's proportions when
// the driver round-robins through them.
const planResolution = 1000

// queryPlan is a spec-driven, weight-expanded schedule. The query driver indexes
// into queries and pageSizes per iteration so the emitted request stream mirrors
// the spec's operation-type and page-size distributions.
//
// The spec is derived from KubeArchive's Kubernetes-style API (namespaces/kind
// paths). Tekton Results exposes a different surface (Results/Records with CEL
// filters), so each spec operation is mapped to the closest Results query. Native
// K8s-style replay is out of scope until that API lands (epic Stories 14–16).
type queryPlan struct {
	queries   []query
	pageSizes []int32
}

// buildQuerySchedule returns the query set, page-size sequence, and namespace
// index sequence the driver round-robins through. With legacy set, it reproduces
// the historic hardcoded mix at a fixed page size and even namespace spread;
// otherwise it derives everything from the query-mix spec (embedded default when
// specPath is empty).
func buildQuerySchedule(specPath string, legacy bool, pageSize int32, nsCount int) ([]query, []int32, []int, error) {
	if legacy {
		return defaultQueries(), []int32{pageSize}, identitySequence(nsCount), nil
	}
	spec, err := loadQuerySpec(specPath)
	if err != nil {
		return nil, nil, nil, err
	}
	plan, err := planFromSpec(spec)
	if err != nil {
		return nil, nil, nil, err
	}
	return plan.queries, plan.pageSizes, namespaceSequence(nsCount, spec.NamespaceFanout), nil
}

// loadQuerySpec loads the spec from path, or the embedded committed spec when path
// is empty.
func loadQuerySpec(path string) (*workload.QueryMixSpec, error) {
	if path == "" {
		return workload.Default()
	}
	data, err := os.ReadFile(path) //nolint:gosec // operator-provided spec path
	if err != nil {
		return nil, err
	}
	return workload.Load(data)
}

// planFromSpec expands a validated spec into a weighted query/page-size schedule.
func planFromSpec(spec *workload.QueryMixSpec) (queryPlan, error) {
	if err := spec.Validate(); err != nil {
		return queryPlan{}, err
	}
	var p queryPlan
	labelKey := topLabelKey(spec)
	for _, op := range spec.Operations {
		q := queryForOp(op, labelKey)
		for i := 0; i < expandCount(op.Weight); i++ {
			p.queries = append(p.queries, q)
		}
	}
	for _, ps := range spec.PageSizes {
		if ps.Limit <= 0 || ps.Limit > math.MaxInt32 {
			continue
		}
		limit := int32(ps.Limit)
		for i := 0; i < expandCount(ps.Weight); i++ {
			p.pageSizes = append(p.pageSizes, limit)
		}
	}
	if len(p.queries) == 0 {
		return queryPlan{}, fmt.Errorf("query-mix spec produced no queries")
	}
	if len(p.pageSizes) == 0 {
		p.pageSizes = []int32{1000}
	}
	return p, nil
}

// queryForOp maps one spec operation to the closest Results/Records query.
func queryForOp(op workload.OperationWeight, labelKey string) query {
	dt := dataTypeFilter(op.Kind)
	switch op.Type {
	case workload.OpLabelSelectorList:
		filter := dt
		if labelKey != "" {
			// Values are anonymized out of the spec, so we exercise the label-index
			// path with an existence predicate rather than an invented value.
			filter = fmt.Sprintf(`%s && data.metadata.labels[%q] != ""`, dt, labelKey)
		}
		return query{Name: "spec-label-list", Kind: kindRecords, Op: "list_records_label", Parent: "%s/results/-", Filter: filter}
	case workload.OpGetByName:
		return query{Name: "spec-get-by-name", Kind: kindRecords, Op: "get_record_by_name", Parent: "%s/results/-", Filter: dt, Single: true}
	case workload.OpOwnerUIDLookup:
		return query{Name: "spec-owner-uid", Kind: kindRecords, Op: "owner_uid_lookup", Parent: "%s/results/-", Filter: dt, Single: true}
	case workload.OpListByNamespaceKind:
		return query{Name: "spec-list", Kind: kindRecords, Op: "list_records_type", Parent: "%s/results/-", Filter: dt}
	default:
		return query{Name: "spec-list", Kind: kindRecords, Op: "list_records_type", Parent: "%s/results/-", Filter: dt}
	}
}

// dataTypeFilter maps a KubeArchive resource kind to the Records data_type filter.
func dataTypeFilter(kind string) string {
	switch strings.ToLower(kind) {
	case "taskruns", "taskrun":
		return "data_type == TASK_RUN"
	default:
		return "data_type == PIPELINE_RUN"
	}
}

// topLabelKey returns the most frequent label-selector key, or "" if none.
func topLabelKey(spec *workload.QueryMixSpec) string {
	if len(spec.LabelSelector.Keys) == 0 {
		return ""
	}
	return spec.LabelSelector.Keys[0].Key
}

// namespaceSequence returns indices into a namespace pool of size nsCount that
// honor the fan-out: the hottest namespace (index 0) appears with frequency
// TopShare, the rest spread round-robin over the others.
func namespaceSequence(nsCount int, f workload.NamespaceFanout) []int {
	if nsCount <= 1 {
		return []int{0}
	}
	if f.TopShare <= 0 || f.TopShare >= 1 {
		return identitySequence(nsCount)
	}
	hot := int(math.Round(f.TopShare * planResolution))
	if hot < 1 {
		hot = 1
	}
	seq := make([]int, 0, planResolution)
	for i := 0; i < hot; i++ {
		seq = append(seq, 0)
	}
	for i := 0; i < planResolution-hot; i++ {
		seq = append(seq, 1+(i%(nsCount-1)))
	}
	return seq
}

// identitySequence returns [0, 1, …, n-1], or [0] when n <= 0.
func identitySequence(n int) []int {
	if n <= 0 {
		return []int{0}
	}
	seq := make([]int, n)
	for i := range seq {
		seq[i] = i
	}
	return seq
}

// expandCount converts a weight into its slot count on the plan resolution,
// guaranteeing every observed entry appears at least once.
func expandCount(weight float64) int {
	n := int(math.Round(weight * planResolution))
	if n < 1 {
		return 1
	}
	return n
}
