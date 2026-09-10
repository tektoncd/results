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
	"context"
	"fmt"

	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"github.com/tektoncd/results/test/performance/metrics"
)

// queryKind distinguishes the two list surfaces.
type queryKind int

const (
	kindResults queryKind = iota
	kindRecords
)

// query is one predefined read against the seed range. Parent is formatted with
// a namespace so a run exercises the whole namespace pool. Filters use only the
// CEL fields the server exposes (see pkg/api/server/cel/env.go).
type query struct {
	Name   string
	Kind   queryKind
	Op     string // metric op name
	Parent string // fmt template taking the namespace
	Filter string
}

// defaultQueries is the standard read mix: list-all, by-status, recent-window
// (Results) and by-type, by-label (Records).
func defaultQueries() []query {
	return []query{
		{Name: "results-all", Kind: kindResults, Op: "list_results_all", Parent: "%s", Filter: ""},
		{Name: "results-success", Kind: kindResults, Op: "list_results_status", Parent: "%s", Filter: "summary.status == SUCCESS"},
		{Name: "results-recent", Kind: kindResults, Op: "list_results_recent", Parent: "%s", Filter: `summary.end_time > timestamp("2026-01-20T00:00:00Z")`},
		{Name: "records-pipelinerun", Kind: kindRecords, Op: "list_records_type", Parent: "%s/results/-", Filter: "data_type == PIPELINE_RUN"},
		{Name: "records-by-label", Kind: kindRecords, Op: "list_records_label", Parent: "%s/results/-", Filter: `data.metadata.labels["appstudio.openshift.io/component"] == "frontend"`},
	}
}

// lister is the subset of the API used by the query driver, satisfied by the
// REST client directly and by the gRPC client via grpcLister.
type lister interface {
	ListResults(ctx context.Context, in *pb.ListResultsRequest) (*pb.ListResultsResponse, error)
	ListRecords(ctx context.Context, in *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error)
}

// grpcLister adapts the variadic gRPC client to the lister interface.
type grpcLister struct{ c pb.ResultsClient }

func (g grpcLister) ListResults(ctx context.Context, in *pb.ListResultsRequest) (*pb.ListResultsResponse, error) {
	return g.c.ListResults(ctx, in)
}

func (g grpcLister) ListRecords(ctx context.Context, in *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error) {
	return g.c.ListRecords(ctx, in)
}

// runQuery executes one query with full pagination, recording per-page latency
// and errors. maxPages guards against a server returning a non-terminating token.
func runQuery(ctx context.Context, l lister, q query, namespace string, pageSize int32, m *metrics.MetricSet) {
	const maxPages = 100000
	parent := fmt.Sprintf(q.Parent, namespace)
	token := ""

	for page := 0; page < maxPages; page++ {
		var next string
		err := observe(m, q.Op, func() error {
			var e error
			next, e = listPage(ctx, l, q, parent, pageSize, token)
			return e
		})
		if err != nil || next == "" || next == token {
			return
		}
		token = next
	}
}

// listPage issues a single page request and returns the next page token.
func listPage(ctx context.Context, l lister, q query, parent string, pageSize int32, token string) (string, error) {
	switch q.Kind {
	case kindResults:
		resp, err := l.ListResults(ctx, &pb.ListResultsRequest{
			Parent:    parent,
			Filter:    q.Filter,
			PageSize:  pageSize,
			PageToken: token,
		})
		if err != nil {
			return "", err
		}
		return resp.GetNextPageToken(), nil
	case kindRecords:
		resp, err := l.ListRecords(ctx, &pb.ListRecordsRequest{
			Parent:    parent,
			Filter:    q.Filter,
			PageSize:  pageSize,
			PageToken: token,
		})
		if err != nil {
			return "", err
		}
		return resp.GetNextPageToken(), nil
	default:
		return "", fmt.Errorf("unknown query kind %d", q.Kind)
	}
}
