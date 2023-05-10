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
	"context"
	"fmt"
	"github.com/tektoncd/results/pkg/api/server/config"
	"github.com/tektoncd/results/pkg/api/server/logger"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/results/pkg/api/server/db/pagination"
	"github.com/tektoncd/results/pkg/api/server/test"
	recordutil "github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestCreateResult(t *testing.T) {
	srv, err := New(&config.Config{DB_ENABLE_AUTO_MIGRATION: true}, logger.Get("info"), test.NewDB(t))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ctx := context.Background()
	req := &pb.CreateResultRequest{
		Parent: "foo",
		Result: &pb.Result{
			Name:        "foo/results/bar",
			Annotations: map[string]string{"foo": "bar"},
		},
	}
	t.Run("success", func(t *testing.T) {
		got, err := srv.CreateResult(ctx, req)
		if err != nil {
			t.Fatalf("could not create result: %v", err)
		}
		got, err = srv.GetResult(ctx, &pb.GetResultRequest{Name: got.GetName()})
		if err != nil {
			t.Fatalf("could not get result from database: %v", err)
		}
		want := proto.Clone(req.GetResult()).(*pb.Result)
		want.Id = fmt.Sprint(lastID)
		want.CreatedTime = timestamppb.New(clock.Now())
		want.UpdatedTime = timestamppb.New(clock.Now())
		want.Etag = mockEtag(lastID, clock.Now().UnixNano())

		if diff := cmp.Diff(got, want, protocmp.Transform()); diff != "" {
			t.Errorf("-want, +got: %s", diff)
		}
	})

	// Errors
	for _, tc := range []struct {
		name string
		req  *pb.CreateResultRequest
		want codes.Code
	}{
		{
			name: "mismatched parent",
			req: &pb.CreateResultRequest{
				Parent: "foo",
				Result: &pb.Result{
					Name: "baz/results/bar",
				},
			},
			want: codes.InvalidArgument,
		},
		{
			name: "missing name",
			req: &pb.CreateResultRequest{
				Parent: "foo",
				Result: &pb.Result{},
			},
			want: codes.InvalidArgument,
		},
		{
			name: "already exists",
			req:  req,
			want: codes.AlreadyExists,
		},
		{
			name: "large name",
			req: &pb.CreateResultRequest{
				Parent: "foo",
				Result: &pb.Result{
					Name: "foo/results/" + strings.Repeat("a", 256),
				},
			},
			want: codes.InvalidArgument,
		},
		{
			name: "large result summary type",
			req: &pb.CreateResultRequest{
				Parent: "foo",
				Result: &pb.Result{
					Name: "foo/results/bar",
					Summary: &pb.RecordSummary{
						Record: "foo/results/bar/records/baz",
						Type:   strings.Repeat("a", 1024),
					},
				},
			},
			want: codes.InvalidArgument,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := srv.CreateResult(ctx, tc.req); status.Code(err) != tc.want {
				t.Fatalf("want: %v, got: %v - %+v", tc.want, status.Code(err), err)
			}
		})
	}
}

func TestUpdateResult(t *testing.T) {
	srv, err := New(&config.Config{DB_ENABLE_AUTO_MIGRATION: true}, logger.Get("info"), test.NewDB(t))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	ctx := context.Background()

	tt := []struct {
		name        string
		requestName string // the `Name` field of the `UpdateResultRequest`
		etag        string
		update      *pb.Result
		// `expect` is the expected result after an update request, it only contains two fields here: `Annotations` and `Etag`.
		// the other fields will be set the same as the automatically created one.
		expect  *pb.Result
		errcode codes.Code
	}{
		{
			name: "success",
			update: &pb.Result{
				Annotations: map[string]string{"foo": "bar"},
				Summary: &pb.RecordSummary{
					Record: "foo/results/bar/records/baz",
					Type:   "bar",
				},
			},
			etag: mockEtag(lastID+1, clock.Now().UnixNano()),
			expect: &pb.Result{
				Annotations: map[string]string{"foo": "bar"},
				Summary: &pb.RecordSummary{
					Record: "foo/results/bar/records/baz",
					Type:   "bar",
				},
			},
		},
		{
			name:   "test update with empty result",
			expect: &pb.Result{},
		},
		// errors
		{
			name:        "test update with invalid name",
			requestName: "invalid name",
			errcode:     codes.InvalidArgument,
		},
		{
			name:        "test update a non-existent result",
			requestName: "foo/results/bar-non-existent",
			errcode:     codes.NotFound,
		},
		{
			name:    "test update with invalid etag",
			etag:    "invalid etag",
			errcode: codes.FailedPrecondition,
		},
		{
			name:    "result summary with no record/type",
			update:  &pb.Result{Summary: &pb.RecordSummary{}},
			errcode: codes.InvalidArgument,
		},
	}
	for idx, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			// create a result for test.
			created, err := srv.CreateResult(ctx, &pb.CreateResultRequest{
				Parent: "foo",
				Result: &pb.Result{Name: fmt.Sprintf("foo/results/bar-%v", idx)}})
			if err != nil {
				t.Fatalf("could not create result: %v", err)
			}

			// forward the time to test if the UpdateTime field is properly updated.
			fakeClock.Advance(time.Second)

			if tc.requestName == "" {
				tc.requestName = created.GetName()
			}
			updated, err := srv.UpdateResult(ctx, &pb.UpdateResultRequest{Result: tc.update, Name: tc.requestName, Etag: tc.etag})
			if err != nil || tc.errcode != codes.OK {
				if status.Code(err) == tc.errcode {
					return
				}
				t.Fatalf("UpdateResult()=(%v, %v); want %v", updated, err, tc.errcode)
			}

			proto.Merge(tc.expect, created)
			tc.expect.UpdatedTime = timestamppb.New(clock.Now())
			tc.expect.UpdateTime = timestamppb.New(clock.Now())
			tc.expect.Etag = mockEtag(lastID, clock.Now().UnixNano())

			// test if the returned result is the same as the expected.
			if diff := cmp.Diff(tc.expect, updated, protocmp.Transform()); diff != "" {
				t.Fatalf("-want, +updated: %s", diff)
			}

			// test if the result is successfully updated to the database.
			got, err := srv.GetResult(ctx, &pb.GetResultRequest{Name: updated.GetName()})
			if err != nil {
				t.Fatalf("failed to get result from server: %v", err)
			}
			if diff := cmp.Diff(tc.expect, got, protocmp.Transform()); diff != "" {
				t.Fatalf("-want, +got: %s", diff)
			}
		})
	}
}

func TestGetResult(t *testing.T) {
	srv, err := New(&config.Config{DB_ENABLE_AUTO_MIGRATION: true}, logger.Get("info"), test.NewDB(t))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ctx := context.Background()
	create, err := srv.CreateResult(ctx, &pb.CreateResultRequest{
		Parent: "foo",
		Result: &pb.Result{
			Name: "foo/results/bar",
		},
	})
	if err != nil {
		t.Fatalf("could not create result: %v", err)
	}

	get, err := srv.GetResult(ctx, &pb.GetResultRequest{Name: create.GetName()})
	if err != nil {
		t.Fatalf("could not get result: %v", err)
	}
	if diff := cmp.Diff(create, get, protocmp.Transform()); diff != "" {
		t.Errorf("-want, +got: %s", diff)
	}

	// Errors
	for _, tc := range []struct {
		name string
		req  *pb.GetResultRequest
		want codes.Code
	}{
		{
			name: "no name",
			req:  &pb.GetResultRequest{},
			want: codes.InvalidArgument,
		},
		{
			name: "not found",
			req:  &pb.GetResultRequest{Name: "a/results/doesnotexist"},
			want: codes.NotFound,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := srv.GetResult(ctx, tc.req); status.Code(err) != tc.want {
				t.Fatalf("want: %v, got: %v - %+v", tc.want, status.Code(err), err)
			}
		})
	}
}

func TestDeleteResult(t *testing.T) {
	srv, err := New(&config.Config{DB_ENABLE_AUTO_MIGRATION: true}, logger.Get("info"), test.NewDB(t))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	ctx := context.Background()
	r, err := srv.CreateResult(ctx, &pb.CreateResultRequest{
		Parent: "foo",
		Result: &pb.Result{
			Name: "foo/results/bar",
		},
	})
	if err != nil {
		t.Fatalf("could not create result: %v", err)
	}

	t.Run("success", func(t *testing.T) {
		// Delete inserted taskrun
		if _, err := srv.DeleteResult(ctx, &pb.DeleteResultRequest{Name: r.GetName()}); err != nil {
			t.Fatalf("could not delete taskrun: %v", err)
		}

		// Check if the taskrun is deleted
		if r, err := srv.GetResult(ctx, &pb.GetResultRequest{Name: r.GetName()}); err == nil {
			t.Fatalf("expected result to be deleted, got: %+v", r)
		}
	})

	t.Run("already deleted", func(t *testing.T) {
		// Check if a deleted taskrun can be deleted again
		if _, err := srv.DeleteResult(ctx, &pb.DeleteResultRequest{Name: r.GetName()}); status.Code(err) != codes.NotFound {
			t.Fatalf("expected NOT_FOUND, got: %v", err)
		}
	})
}

func TestCascadeDelete(t *testing.T) {
	srv, err := New(&config.Config{DB_ENABLE_AUTO_MIGRATION: true}, logger.Get("info"), test.NewDB(t))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ctx := context.Background()
	result, err := srv.CreateResult(ctx, &pb.CreateResultRequest{
		Parent: "foo",
		Result: &pb.Result{
			Name: "foo/results/bar",
		},
	})
	if err != nil {
		t.Fatalf("CreateResult: %v", err)
	}
	r, err := srv.CreateRecord(ctx, &pb.CreateRecordRequest{
		Parent: result.GetName(),
		Record: &pb.Record{
			Name: recordutil.FormatName(result.GetName(), "baz"),
		},
	})
	if err != nil {
		t.Fatalf("CreateRecord(): %v", err)
	}
	if _, err := srv.DeleteResult(ctx, &pb.DeleteResultRequest{Name: result.GetName()}); err != nil {
		t.Fatalf("could not delete the result: %v", err)
	}
	if got, err := srv.GetRecord(ctx, &pb.GetRecordRequest{Name: r.GetName()}); status.Code(err) != codes.NotFound {
		t.Fatalf("cascade delete failed - expected Record to be deleted, got: (%+v, %v)", got, err)
	}
}

func TestListResults(t *testing.T) {
	// Reset so IDs match names
	lastID = 0

	// Create a temporary database
	srv, err := New(&config.Config{DB_ENABLE_AUTO_MIGRATION: true}, logger.Get("info"), test.NewDB(t))
	if err != nil {
		t.Fatalf("failed to setup db: %v", err)
	}
	ctx := context.Background()

	parent := "foo"
	results := make([]*pb.Result, 0, 5)

	for i := 1; i <= cap(results); i++ {
		fakeClock.Advance(time.Second)
		res, err := srv.CreateResult(ctx, &pb.CreateResultRequest{
			Parent: "foo",
			Result: &pb.Result{
				Name:        fmt.Sprintf("%s/results/%d", parent, i),
				Annotations: map[string]string{"foo": fmt.Sprintf("bar-%d", i)},
			},
		})
		if err != nil {
			t.Fatalf("could not create result: %v", err)
		}
		t.Logf("Created name: %s, id: %s", res.GetName(), res.GetId())
		results = append(results, res)
	}

	reversedResults := make([]*pb.Result, len(results))
	for i := len(results); i > 0; i-- {
		reversedResults[len(results)-i] = results[i-1]
	}

	tt := []struct {
		name   string
		req    *pb.ListResultsRequest
		want   *pb.ListResultsResponse
		status codes.Code
	}{
		{
			name: "list all",
			req: &pb.ListResultsRequest{
				Parent: parent,
			},
			want: &pb.ListResultsResponse{
				Results: results,
			},
			status: codes.OK,
		},
		{
			name: "list all results without knowing the parent name",
			req: &pb.ListResultsRequest{
				Parent: "-",
			},
			want: &pb.ListResultsResponse{
				Results: results,
			},
			status: codes.OK,
		},
		{
			name: "list all w/ pagination token",
			req: &pb.ListResultsRequest{
				Parent:   parent,
				PageSize: int32(len(results)),
			},
			want: &pb.ListResultsResponse{
				Results: results,
			},
			status: codes.OK,
		},
		{
			name: "no results",
			req: &pb.ListResultsRequest{
				Parent: fmt.Sprintf("%s-doesnotexist", parent),
			},
			want:   &pb.ListResultsResponse{},
			status: codes.OK,
		},
		{
			name:   "missing parent",
			req:    &pb.ListResultsRequest{},
			status: codes.InvalidArgument,
		},
		{
			name: "simple query",
			req: &pb.ListResultsRequest{
				Parent: parent,
				Filter: `result.id == "1"`,
			},
			want: &pb.ListResultsResponse{
				Results: results[:1],
			},
		},
		{
			name: "simple query - function",
			req: &pb.ListResultsRequest{
				Parent: parent,
				Filter: `result.id.endsWith("1")`,
			},
			want: &pb.ListResultsResponse{
				Results: results[:1],
			},
		},
		{
			name: "complex query",
			req: &pb.ListResultsRequest{
				Parent: parent,
				Filter: `result.id == "1" || result.id == "2"`,
			},
			want: &pb.ListResultsResponse{
				Results: results[:2],
			},
		},
		{
			name: "filter all",
			req: &pb.ListResultsRequest{
				Parent: parent,
				Filter: `result.id == "doesnotexist"`,
			},
			want: &pb.ListResultsResponse{},
		},
		{
			name: "filter by annotations",
			req: &pb.ListResultsRequest{
				Parent: parent,
				Filter: `result.annotations["foo"]=="bar-1"`,
			},
			want: &pb.ListResultsResponse{
				Results: results[:1],
			},
		},
		{
			name: "non-boolean expression",
			req: &pb.ListResultsRequest{
				Parent: parent,
				Filter: `result.id`,
			},
			status: codes.InvalidArgument,
		},
		{
			name: "wrong resource type",
			req: &pb.ListResultsRequest{
				Parent: parent,
				Filter: `taskrun.api_version != ""`,
			},
			status: codes.InvalidArgument,
		},
		{
			name: "partial response",
			req: &pb.ListResultsRequest{
				Parent:   parent,
				PageSize: 1,
			},
			want: &pb.ListResultsResponse{
				Results:       results[:1],
				NextPageToken: pagetoken(t, results[1].GetId(), ""),
			},
		},
		{
			name: "partial response with filter",
			req: &pb.ListResultsRequest{
				Parent:   parent,
				PageSize: 1,
				Filter:   `result.id > "1"`,
			},
			want: &pb.ListResultsResponse{
				Results:       results[1:2],
				NextPageToken: pagetoken(t, results[2].GetId(), `result.id > "1"`),
			},
		},
		{
			name: "with page token",
			req: &pb.ListResultsRequest{
				Parent:    parent,
				PageToken: pagetoken(t, results[0].GetId(), ""),
			},
			want: &pb.ListResultsResponse{
				Results: results[1:],
			},
		},
		{
			name: "with page token and filter and page size",
			req: &pb.ListResultsRequest{
				Parent:    parent,
				PageToken: pagetoken(t, results[0].GetId(), `result.id > "1"`),
				Filter:    `result.id > "1"`,
				PageSize:  1,
			},
			want: &pb.ListResultsResponse{
				Results:       results[1:2],
				NextPageToken: pagetoken(t, results[2].GetId(), `result.id > "1"`),
			},
		},
		{
			name: "invalid page size",
			req: &pb.ListResultsRequest{
				Parent:   parent,
				PageSize: -1,
			},
			status: codes.InvalidArgument,
		},
		// Order By
		{
			name: "with order by desc",
			req: &pb.ListResultsRequest{
				Parent:  parent,
				OrderBy: `created_time desc`,
			},
			want: &pb.ListResultsResponse{
				Results: reversedResults,
			},
		},
		{
			name: "with order by asc",
			req: &pb.ListResultsRequest{
				Parent:  parent,
				OrderBy: `created_time asc`,
			},
			want: &pb.ListResultsResponse{
				Results: results,
			},
		},
		{
			name: "with default order by direction",
			req: &pb.ListResultsRequest{
				Parent:  parent,
				OrderBy: `created_time`,
			},
			want: &pb.ListResultsResponse{
				Results: results,
			},
		},
		{
			name: "with invalid order field name",
			req: &pb.ListResultsRequest{
				Parent:  parent,
				OrderBy: `name`,
			},
			status: codes.InvalidArgument,
		},
		{
			name: "with invalid order clause",
			req: &pb.ListResultsRequest{
				Parent:  parent,
				OrderBy: `created_time asc foo`,
			},
			status: codes.InvalidArgument,
		},
		{
			name: "with invalid order direction",
			req: &pb.ListResultsRequest{
				Parent:  parent,
				OrderBy: `created_time foo`,
			},
			status: codes.InvalidArgument,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			got, err := srv.ListResults(ctx, tc.req)
			if status.Code(err) != tc.status {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("-want,+got: %s", diff)
				if name, filter, err := pagination.DecodeToken(got.GetNextPageToken()); err == nil {
					t.Logf("Next (name, filter) = (%s, %s)", name, filter)
				}
			}
		})
	}
}

func pagetoken(t *testing.T, name, filter string) string {
	t.Helper()

	if token, err := pagination.EncodeToken(name, filter); err != nil {
		t.Fatalf("Failed to get encoded token: %v", err)
		return ""
	} else {
		return token
	}
}

func mockEtag(id uint32, t int64) string {
	return fmt.Sprintf("%v-%v", id, t)
}
