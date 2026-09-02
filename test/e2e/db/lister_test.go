//go:build e2e
// +build e2e

// Copyright 2026 The Tekton Authors
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

package db_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// seedResults creates n Results via the deployed API under the given parent.
// Returns the created Result protos in creation order.
//
// The API requires Summary.Record and Summary.Type whenever Summary is non-nil,
// so we populate them with well-formed placeholders. Result and record names
// must be lowercase per the API's name regex.
func seedResults(t *testing.T, parent string, n int, summaryStatus pb.RecordSummary_Status) []*pb.Result {
	t.Helper()
	ctx := context.Background()
	results := make([]*pb.Result, 0, n)

	statusName := strings.ToLower(pb.RecordSummary_Status_name[int32(summaryStatus)])
	for i := 0; i < n; i++ {
		resultID := fmt.Sprintf("lister-%s-%d", statusName, i)
		name := fmt.Sprintf("%s/results/%s", parent, resultID)
		recordName := fmt.Sprintf("%s/records/summary", name)
		res, err := adminClient.CreateResult(ctx, &pb.CreateResultRequest{
			Parent: parent,
			Result: &pb.Result{
				Name: name,
				Annotations: map[string]string{
					"index":  fmt.Sprintf("%d", i),
					"series": statusName,
				},
				Summary: &pb.RecordSummary{
					Record: recordName,
					Type:   "testing.tekton.dev/test",
					Status: summaryStatus,
				},
			},
		})
		if err != nil {
			t.Fatalf("seedResults: CreateResult(%s) failed: %v", name, err)
		}
		results = append(results, res)
	}
	return results
}

// cleanupParent deletes all Results under the given parent via the API.
func cleanupParent(t *testing.T, parent string) {
	t.Helper()
	ctx := context.Background()
	resp, err := adminClient.ListResults(ctx, &pb.ListResultsRequest{Parent: parent})
	if err != nil {
		return
	}
	for _, r := range resp.Results {
		_, _ = adminClient.DeleteResult(ctx, &pb.DeleteResultRequest{Name: r.GetName()})
	}
}

// TestLister_BasicList verifies ListResults returns all Results under a parent.
func TestLister_BasicList(t *testing.T) {
	const parent = "db-list-basic"
	t.Cleanup(func() { cleanupParent(t, parent) })
	seeded := seedResults(t, parent, 6, pb.RecordSummary_SUCCESS)

	resp, err := readClient.ListResults(defaultCtx(), &pb.ListResultsRequest{
		Parent: parent,
	})
	if err != nil {
		t.Fatalf("ListResults failed: %v", err)
	}

	if got := len(resp.Results); got != len(seeded) {
		t.Errorf("want %d results, got %d", len(seeded), got)
	}

	// Verify every seeded name appears in the response.
	gotNames := make(map[string]bool, len(resp.Results))
	for _, r := range resp.Results {
		gotNames[r.Name] = true
	}
	for _, s := range seeded {
		if !gotNames[s.Name] {
			t.Errorf("seeded Result %q not in ListResults response", s.Name)
		}
	}
}

// TestLister_WildcardParent verifies that parent="-" returns Results across
// all namespaces.
func TestLister_WildcardParent(t *testing.T) {
	const parentA = "db-list-wc-a"
	const parentB = "db-list-wc-b"
	t.Cleanup(func() {
		cleanupParent(t, parentA)
		cleanupParent(t, parentB)
	})

	seededA := seedResults(t, parentA, 3, pb.RecordSummary_SUCCESS)
	seededB := seedResults(t, parentB, 3, pb.RecordSummary_FAILURE)

	resp, err := readClient.ListResults(defaultCtx(), &pb.ListResultsRequest{
		Parent: "-",
	})
	if err != nil {
		t.Fatalf("ListResults with wildcard parent failed: %v", err)
	}

	gotNames := make(map[string]bool, len(resp.Results))
	for _, r := range resp.Results {
		gotNames[r.Name] = true
	}

	for _, s := range seededA {
		if !gotNames[s.Name] {
			t.Errorf("seeded Result %q from parent %s not found with wildcard parent", s.Name, parentA)
		}
	}
	for _, s := range seededB {
		if !gotNames[s.Name] {
			t.Errorf("seeded Result %q from parent %s not found with wildcard parent", s.Name, parentB)
		}
	}
}

// TestLister_FilterByStatus verifies the CEL→SQL filter summary.status == SUCCESS
// executes correctly on Postgres.
func TestLister_FilterByStatus(t *testing.T) {
	const parent = "db-list-filter"
	t.Cleanup(func() { cleanupParent(t, parent) })

	seedResults(t, parent, 5, pb.RecordSummary_SUCCESS)
	seedResults(t, parent, 4, pb.RecordSummary_FAILURE)

	resp, err := readClient.ListResults(defaultCtx(), &pb.ListResultsRequest{
		Parent: parent,
		Filter: "summary.status == SUCCESS",
	})
	if err != nil {
		t.Fatalf("ListResults with filter failed: %v", err)
	}

	if got := len(resp.Results); got != 5 {
		t.Errorf("want 5 SUCCESS results, got %d", got)
	}
	for _, r := range resp.Results {
		if r.Summary.Status != pb.RecordSummary_SUCCESS {
			t.Errorf("result %s leaked non-SUCCESS status %v", r.Name, r.Summary.Status)
		}
	}
}

// TestLister_FilterNotEqual verifies the != CEL operator.
func TestLister_FilterNotEqual(t *testing.T) {
	const parent = "db-list-neq"
	t.Cleanup(func() { cleanupParent(t, parent) })

	seedResults(t, parent, 3, pb.RecordSummary_SUCCESS)
	seedResults(t, parent, 4, pb.RecordSummary_FAILURE)

	resp, err := readClient.ListResults(defaultCtx(), &pb.ListResultsRequest{
		Parent: parent,
		Filter: "summary.status != SUCCESS",
	})
	if err != nil {
		t.Fatalf("ListResults with != filter failed: %v", err)
	}

	if got := len(resp.Results); got != 4 {
		t.Errorf("want 4 non-SUCCESS results, got %d", got)
	}
}

// TestLister_OrderByCreateTimeDesc verifies descending ORDER BY on Postgres.
func TestLister_OrderByCreateTimeDesc(t *testing.T) {
	const parent = "db-list-order-desc"
	t.Cleanup(func() { cleanupParent(t, parent) })
	seedResults(t, parent, 6, pb.RecordSummary_SUCCESS)

	resp, err := readClient.ListResults(defaultCtx(), &pb.ListResultsRequest{
		Parent:  parent,
		OrderBy: "create_time desc",
	})
	if err != nil {
		t.Fatalf("ListResults with ORDER BY failed: %v", err)
	}

	for i := 1; i < len(resp.Results); i++ {
		prev := resp.Results[i-1].CreateTime.AsTime()
		curr := resp.Results[i].CreateTime.AsTime()
		if prev.Before(curr) {
			t.Errorf("result[%d] create_time (%v) < result[%d] (%v); want DESC",
				i-1, prev, i, curr)
		}
	}
}

// TestLister_OrderByCreateTimeAsc verifies ascending ORDER BY on Postgres.
func TestLister_OrderByCreateTimeAsc(t *testing.T) {
	const parent = "db-list-order-asc"
	t.Cleanup(func() { cleanupParent(t, parent) })
	seedResults(t, parent, 6, pb.RecordSummary_SUCCESS)

	resp, err := readClient.ListResults(defaultCtx(), &pb.ListResultsRequest{
		Parent:  parent,
		OrderBy: "create_time asc",
	})
	if err != nil {
		t.Fatalf("ListResults with ORDER BY ASC failed: %v", err)
	}

	for i := 1; i < len(resp.Results); i++ {
		prev := resp.Results[i-1].CreateTime.AsTime()
		curr := resp.Results[i].CreateTime.AsTime()
		if prev.After(curr) {
			t.Errorf("result[%d] create_time (%v) > result[%d] (%v); want ASC",
				i-1, prev, i, curr)
		}
	}
}

// TestLister_Pagination verifies keyset pagination on Postgres. Uses
// pageSize=5 (the minimum allowed by the lister). Checks per-page size,
// total count, and no duplicates.
func TestLister_Pagination(t *testing.T) {
	const parent = "db-list-page"
	const total = 13
	const pageSize int32 = 5
	t.Cleanup(func() { cleanupParent(t, parent) })
	seedResults(t, parent, total, pb.RecordSummary_SUCCESS)

	var allNames []string
	pageToken := ""

	for page := 0; ; page++ {
		resp, err := readClient.ListResults(defaultCtx(), &pb.ListResultsRequest{
			Parent:    parent,
			PageSize:  pageSize,
			PageToken: pageToken,
		})
		if err != nil {
			t.Fatalf("page %d: ListResults failed: %v", page, err)
		}

		if resp.NextPageToken != "" && int32(len(resp.Results)) != pageSize {
			t.Errorf("page %d: non-final page has %d items, want %d",
				page, len(resp.Results), pageSize)
		}

		for _, r := range resp.Results {
			allNames = append(allNames, r.Name)
		}

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken

		if page > 20 {
			t.Fatal("pagination did not terminate after 20 pages")
		}
	}

	if got := len(allNames); got != total {
		t.Errorf("pagination returned %d results, want %d", got, total)
	}

	seen := make(map[string]bool)
	for _, name := range allNames {
		if seen[name] {
			t.Errorf("duplicate result in pagination: %s", name)
		}
		seen[name] = true
	}
}

// TestLister_PaginationWithFilterAndOrder exercises the full query pipeline
// (filter + order + pagination) on Postgres.
func TestLister_PaginationWithFilterAndOrder(t *testing.T) {
	const parent = "db-list-combo"
	t.Cleanup(func() { cleanupParent(t, parent) })
	seedResults(t, parent, 12, pb.RecordSummary_SUCCESS)
	seedResults(t, parent, 6, pb.RecordSummary_FAILURE)

	var successNames []string
	pageToken := ""

	for page := 0; ; page++ {
		resp, err := readClient.ListResults(defaultCtx(), &pb.ListResultsRequest{
			Parent:    parent,
			Filter:    "summary.status == SUCCESS",
			OrderBy:   "create_time desc",
			PageSize:  5,
			PageToken: pageToken,
		})
		if err != nil {
			t.Fatalf("page %d: ListResults failed: %v", page, err)
		}

		for _, r := range resp.Results {
			if r.Summary.Status != pb.RecordSummary_SUCCESS {
				t.Errorf("filter leaked non-SUCCESS result: %s (status=%v)", r.Name, r.Summary.Status)
			}
			successNames = append(successNames, r.Name)
		}

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken

		if page > 20 {
			t.Fatal("pagination did not terminate")
		}
	}

	if got := len(successNames); got != 12 {
		t.Errorf("want 12 SUCCESS results, got %d", got)
	}
}

// TestLister_EmptyResult verifies that a filter matching nothing returns an
// empty list with no page token.
func TestLister_EmptyResult(t *testing.T) {
	const parent = "db-list-empty"
	t.Cleanup(func() { cleanupParent(t, parent) })
	seedResults(t, parent, 5, pb.RecordSummary_SUCCESS)

	resp, err := readClient.ListResults(defaultCtx(), &pb.ListResultsRequest{
		Parent: parent,
		Filter: "summary.status == FAILURE",
	})
	if err != nil {
		t.Fatalf("ListResults failed: %v", err)
	}
	if got := len(resp.Results); got != 0 {
		t.Errorf("want 0 results, got %d", got)
	}
	if resp.NextPageToken != "" {
		t.Errorf("want empty NextPageToken, got %q", resp.NextPageToken)
	}
}

// TestLister_ListRecords verifies ListRecords against the deployed Postgres.
func TestLister_ListRecords(t *testing.T) {
	const parent = "db-list-records"
	t.Cleanup(func() { cleanupParent(t, parent) })

	res, err := adminClient.CreateResult(defaultCtx(), &pb.CreateResultRequest{
		Parent: parent,
		Result: &pb.Result{Name: parent + "/results/with-records"},
	})
	if err != nil {
		t.Fatalf("CreateResult failed: %v", err)
	}

	const numRecords = 7
	createdNames := make([]string, 0, numRecords)
	for i := 0; i < numRecords; i++ {
		recName := fmt.Sprintf("%s/records/rec-%d", res.GetName(), i)
		rec, err := adminClient.CreateRecord(defaultCtx(), &pb.CreateRecordRequest{
			Parent: res.GetName(),
			Record: &pb.Record{
				Name: recName,
				Data: &pb.Any{
					Type:  "testing.tekton.dev/test",
					Value: mustJSON(t, map[string]string{"i": fmt.Sprintf("%d", i)}),
				},
			},
		})
		if err != nil {
			t.Fatalf("CreateRecord(%s) failed: %v", recName, err)
		}
		createdNames = append(createdNames, rec.GetName())
	}

	resp, err := readClient.ListRecords(defaultCtx(), &pb.ListRecordsRequest{
		Parent: res.GetName(),
	})
	if err != nil {
		t.Fatalf("ListRecords failed: %v", err)
	}

	gotNames := make(map[string]bool, len(resp.Records))
	for _, r := range resp.Records {
		gotNames[r.GetName()] = true
	}
	for _, name := range createdNames {
		if !gotNames[name] {
			t.Errorf("created Record %q not in ListRecords response", name)
		}
	}
}

// TestLister_InvalidFilter verifies invalid CEL returns InvalidArgument.
func TestLister_InvalidFilter(t *testing.T) {
	_, err := readClient.ListResults(defaultCtx(), &pb.ListResultsRequest{
		Parent: "db-list-invalid",
		Filter: "this is not a valid CEL expression !!!",
	})
	if err == nil {
		t.Fatal("expected error for invalid filter, got nil")
	}
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("want gRPC code %s, got %s (err: %v)", codes.InvalidArgument, got, err)
	}
}

// TestLister_InvalidOrderBy verifies unsupported order_by returns InvalidArgument.
func TestLister_InvalidOrderBy(t *testing.T) {
	_, err := readClient.ListResults(defaultCtx(), &pb.ListResultsRequest{
		Parent:  "db-list-invalid-order",
		OrderBy: "nonexistent_field",
	})
	if err == nil {
		t.Fatal("expected error for invalid order_by, got nil")
	}
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("want gRPC code %s, got %s (err: %v)", codes.InvalidArgument, got, err)
	}
}
