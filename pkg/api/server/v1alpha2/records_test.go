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
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/results/pkg/api/server/db/pagination"
	"github.com/tektoncd/results/pkg/api/server/test"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	recordutil "github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/result"
	resultutil "github.com/tektoncd/results/pkg/api/server/v1alpha2/result"
	"github.com/tektoncd/results/pkg/internal/jsonutil"
	ppb "github.com/tektoncd/results/proto/pipeline/v1beta1/pipeline_go_proto"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCreateRecord(t *testing.T) {
	srv, err := New(test.NewDB(t))
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

	req := &pb.CreateRecordRequest{
		Parent: result.GetName(),
		Record: &pb.Record{
			Name: recordutil.FormatName(result.GetName(), "baz"),
			Data: &pb.Any{
				Type:  "TaskRun",
				Value: jsonutil.AnyBytes(t, &v1beta1.TaskRun{ObjectMeta: v1.ObjectMeta{Name: "tacocat"}}),
			},
		},
	}
	t.Run("success", func(t *testing.T) {
		got, err := srv.CreateRecord(ctx, req)
		if err != nil {
			t.Fatalf("CreateRecord: %v", err)
		}
		want := proto.Clone(req.GetRecord()).(*pb.Record)
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
		req  *pb.CreateRecordRequest
		want codes.Code
	}{
		{
			name: "mismatched parent",
			req: &pb.CreateRecordRequest{
				Parent: req.GetParent(),
				Record: &pb.Record{
					Name: resultutil.FormatName("foo", "baz"),
				},
			},
			want: codes.InvalidArgument,
		},
		{
			name: "parent does not exist",
			req: &pb.CreateRecordRequest{
				Parent: resultutil.FormatName("foo", "doesnotexist"),
				Record: &pb.Record{
					Name: recordutil.FormatName(resultutil.FormatName("foo", "doesnotexist"), "baz"),
				},
			},
			want: codes.NotFound,
		},
		{
			name: "missing name",
			req: &pb.CreateRecordRequest{
				Parent: req.GetParent(),
				Record: &pb.Record{
					Name: fmt.Sprintf("%s/results/", result.GetName()),
				},
			},
			want: codes.InvalidArgument,
		},
		{
			name: "result used as name",
			req: &pb.CreateRecordRequest{
				Parent: req.GetParent(),
				Record: &pb.Record{
					Name: result.GetName(),
				},
			},
			want: codes.InvalidArgument,
		},
		{
			name: "already exists",
			req:  req,
			want: codes.AlreadyExists,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := srv.CreateRecord(ctx, tc.req); status.Code(err) != tc.want {
				t.Fatalf("want: %v, got: %v - %+v", tc.want, status.Code(err), err)
			}
		})
	}
}

// TestCreateRecord_ConcurrentDelete simulates a concurrent deletion of a
// Result parent mocking the result name -> id conversion. This tricks the
// API Server into thinking the parent is valid during initial validation,
// but fails when writing the Record due to foreign key constraints.
func TestCreateRecord_ConcurrentDelete(t *testing.T) {
	result := "deleted"
	srv, err := New(
		test.NewDB(t),
		withGetResultID(func(context.Context, string, string) (string, error) {
			return result, nil
		}),
	)
	if err != nil {
		t.Fatalf("error creating server: %v", err)
	}

	ctx := context.Background()
	parent := resultutil.FormatName("foo", result)
	record, err := srv.CreateRecord(ctx, &pb.CreateRecordRequest{
		Parent: parent,
		Record: &pb.Record{
			Name: recordutil.FormatName(parent, "baz"),
		},
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("CreateRecord: %+v, %v", record, err)
	}
}

func TestGetRecord(t *testing.T) {
	srv, err := New(test.NewDB(t))
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

	record, err := srv.CreateRecord(ctx, &pb.CreateRecordRequest{
		Parent: result.GetName(),
		Record: &pb.Record{
			Name: recordutil.FormatName(result.GetName(), "baz"),
		},
	})
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}

	t.Run("success", func(t *testing.T) {
		got, err := srv.GetRecord(ctx, &pb.GetRecordRequest{Name: record.GetName()})
		if err != nil {
			t.Fatalf("GetRecord: %v", err)
		}
		if diff := cmp.Diff(got, record, protocmp.Transform()); diff != "" {
			t.Errorf("-want, +got: %s", diff)
		}
	})

	// Errors
	for _, tc := range []struct {
		name string
		req  *pb.GetRecordRequest
		want codes.Code
	}{
		{
			name: "no name",
			req:  &pb.GetRecordRequest{},
			want: codes.InvalidArgument,
		},
		{
			name: "invalid name",
			req:  &pb.GetRecordRequest{Name: "a/results/doesnotexist"},
			want: codes.InvalidArgument,
		},
		{
			name: "not found",
			req:  &pb.GetRecordRequest{Name: recordutil.FormatName(result.GetName(), "doesnotexist")},
			want: codes.NotFound,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := srv.GetRecord(ctx, tc.req); status.Code(err) != tc.want {
				t.Fatalf("want: %v, got: %v - %+v", tc.want, status.Code(err), err)
			}
		})
	}
}

func TestListRecords(t *testing.T) {
	// Create a temporary database
	srv, err := New(test.NewDB(t))
	if err != nil {
		t.Fatalf("failed to setup db: %v", err)
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

	records := make([]*pb.Record, 0, 6)
	// Create 3 TaskRun records
	for i := 0; i < 3; i++ {
		fakeClock.Advance(time.Second)
		r, err := srv.CreateRecord(ctx, &pb.CreateRecordRequest{
			Parent: result.GetName(),
			Record: &pb.Record{
				Name: fmt.Sprintf("%s/records/%d", result.GetName(), i),
				Data: &pb.Any{
					Type: "TaskRun",
					Value: jsonutil.AnyBytes(t, &v1beta1.TaskRun{ObjectMeta: v1.ObjectMeta{
						Name: fmt.Sprintf("%d", i),
					}}),
				},
			},
		})
		if err != nil {
			t.Fatalf("could not create result: %v", err)
		}
		t.Logf("Created record: %+v", r)
		records = append(records, r)
	}

	// Create 3 PipelineRun records
	for i := 3; i < 6; i++ {
		fakeClock.Advance(time.Second)
		r, err := srv.CreateRecord(ctx, &pb.CreateRecordRequest{
			Parent: result.GetName(),
			Record: &pb.Record{
				Name: fmt.Sprintf("%s/records/%d", result.GetName(), i),
				Data: &pb.Any{
					Type: "PipelineRun",
					Value: jsonutil.AnyBytes(t, &v1beta1.PipelineRun{ObjectMeta: v1.ObjectMeta{
						Name: fmt.Sprintf("%d", i),
					}}),
				},
			},
		})
		if err != nil {
			t.Fatalf("could not create result: %v", err)
		}
		t.Logf("Created record: %+v", r)
		records = append(records, r)
	}

	reversedRecords := make([]*pb.Record, len(records))
	for i := len(reversedRecords); i > 0; i-- {
		reversedRecords[len(records)-i] = records[i-1]
	}

	tt := []struct {
		name   string
		req    *pb.ListRecordsRequest
		want   *pb.ListRecordsResponse
		status codes.Code
	}{
		{
			name: "all",
			req: &pb.ListRecordsRequest{
				Parent: result.GetName(),
			},
			want: &pb.ListRecordsResponse{
				Records: records,
			},
		},
		{
			// TODO: We should return NOT_FOUND in the future.
			name: "missing parent",
			req: &pb.ListRecordsRequest{
				Parent: "foo/results/baz",
			},
			want: &pb.ListRecordsResponse{},
		},
		{
			name: "filter by record property",
			req: &pb.ListRecordsRequest{
				Parent: result.GetName(),
				Filter: `name == "foo/results/bar/records/0"`,
			},
			want: &pb.ListRecordsResponse{
				Records: records[:1],
			},
		},
		{
			name: "filter by record data",
			req: &pb.ListRecordsRequest{
				Parent: result.GetName(),
				Filter: `data.metadata.name == "0"`,
			},
			want: &pb.ListRecordsResponse{
				Records: records[:1],
			},
		},
		{
			name: "filter by record type",
			req: &pb.ListRecordsRequest{
				Parent: result.GetName(),
				Filter: `data_type == "TaskRun"`,
			},
			want: &pb.ListRecordsResponse{
				Records: records[:3],
			},
		},
		{
			name: "filter by parent",
			req: &pb.ListRecordsRequest{
				Parent: result.GetName(),
				Filter: fmt.Sprintf(`name.startsWith("%s")`, result.GetName()),
			},
			want: &pb.ListRecordsResponse{
				Records: records,
			},
		},
		// Pagination
		{
			name: "filter and page size",
			req: &pb.ListRecordsRequest{
				Parent:   result.GetName(),
				Filter:   `data_type == "TaskRun"`,
				PageSize: 1,
			},
			want: &pb.ListRecordsResponse{
				Records:       records[:1],
				NextPageToken: pagetoken(t, records[1].GetId(), `data_type == "TaskRun"`),
			},
		},
		{
			name: "only page size",
			req: &pb.ListRecordsRequest{
				Parent:   result.GetName(),
				PageSize: 1,
			},
			want: &pb.ListRecordsResponse{
				Records:       records[:1],
				NextPageToken: pagetoken(t, records[1].GetId(), ""),
			},
		},
		// Order By
		{
			name: "with order asc",
			req: &pb.ListRecordsRequest{
				Parent:  result.GetName(),
				OrderBy: "created_time asc",
			},
			want: &pb.ListRecordsResponse{
				Records: records,
			},
		},
		{
			name: "with order desc",
			req: &pb.ListRecordsRequest{
				Parent:  result.GetName(),
				OrderBy: "created_time desc",
			},
			want: &pb.ListRecordsResponse{
				Records: reversedRecords,
			},
		},
		{
			name: "with missing order",
			req: &pb.ListRecordsRequest{
				Parent:  result.GetName(),
				OrderBy: "",
			},
			want: &pb.ListRecordsResponse{
				Records: records,
			},
		},
		{
			name: "with default order",
			req: &pb.ListRecordsRequest{
				Parent:  result.GetName(),
				OrderBy: "created_time",
			},
			want: &pb.ListRecordsResponse{
				Records: records,
			},
		},

		// Errors
		{
			name: "unknown type",
			req: &pb.ListRecordsRequest{
				Parent: result.GetName(),
				Filter: `type(record.data) == tekton.pipeline.v1beta1.Unknown`,
			},
			status: codes.InvalidArgument,
		},
		{
			name: "unknown any field",
			req: &pb.ListRecordsRequest{
				Parent: result.GetName(),
				Filter: `record.data.metadata.unknown == "tacocat"`,
			},
			status: codes.InvalidArgument,
		},
		{
			name: "invalid page size",
			req: &pb.ListRecordsRequest{
				Parent:   result.GetName(),
				PageSize: -1,
			},
			status: codes.InvalidArgument,
		},
		{
			name: "malformed parent",
			req: &pb.ListRecordsRequest{
				Parent: "unknown",
			},
			status: codes.InvalidArgument,
		},
		{
			name: "invalid order by clause",
			req: &pb.ListRecordsRequest{
				Parent:  result.GetName(),
				OrderBy: "created_time desc asc",
			},
			status: codes.InvalidArgument,
		},
		{
			name: "invalid sort direction",
			req: &pb.ListRecordsRequest{
				Parent:  result.GetName(),
				OrderBy: "created_time foo",
			},
			status: codes.InvalidArgument,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			got, err := srv.ListRecords(ctx, tc.req)
			if status.Code(err) != tc.status {
				t.Fatalf("want %v, got %v", tc.status, err)
			}

			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("-want, +got: %s", diff)
				if name, filter, err := pagination.DecodeToken(got.GetNextPageToken()); err == nil {
					t.Logf("Next (name, filter) = (%s, %s)", name, filter)
				}
			}
		})
	}
}

func TestUpdateRecord(t *testing.T) {
	srv, err := New(test.NewDB(t))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	ctx := context.Background()

	result, err := srv.CreateResult(ctx, &pb.CreateResultRequest{
		Parent: "foo",
		Result: &pb.Result{
			Name: result.FormatName("foo", "bar"),
		},
	})
	if err != nil {
		t.Fatalf("CreateResult(): %v", err)
	}

	tr := &ppb.TaskRun{
		Metadata: &ppb.ObjectMeta{
			Name: "taskrun",
		},
	}

	tt := []struct {
		name string
		// Starting Record to create.
		record *pb.Record
		req    *pb.UpdateRecordRequest
		// Expected update diff: expected Record should be merge of
		// record + diff.
		diff   *pb.Record
		status codes.Code
	}{
		{
			name: "success",
			record: &pb.Record{
				Name: record.FormatName(result.GetName(), "a"),
			},
			req: &pb.UpdateRecordRequest{
				Etag: mockEtag(lastID+1, clock.Now().UnixNano()),
				Record: &pb.Record{
					Name: record.FormatName(result.GetName(), "a"),
					Data: &pb.Any{
						Value: jsonutil.AnyBytes(t, tr),
					},
				},
			},
			diff: &pb.Record{
				Data: &pb.Any{
					Value: jsonutil.AnyBytes(t, tr),
				},
			},
		},
		{
			name: "ignored fields",
			record: &pb.Record{
				Name: record.FormatName(result.GetName(), "b"),
			},
			req: &pb.UpdateRecordRequest{
				Record: &pb.Record{
					Name: record.FormatName(result.GetName(), "b"),
					Id:   "ignored",
				},
			},
		},
		// Errors
		{
			name: "rename",
			req: &pb.UpdateRecordRequest{
				Record: &pb.Record{
					Name: record.FormatName(result.GetName(), "doesnotexist"),
					Data: &pb.Any{
						Value: jsonutil.AnyBytes(t, tr),
					},
				},
			},
			status: codes.NotFound,
		},
		{
			name: "bad name",
			req: &pb.UpdateRecordRequest{
				Record: &pb.Record{
					Name: "tacocat",
					Data: &pb.Any{
						Value: jsonutil.AnyBytes(t, tr),
					},
				},
			},
			status: codes.InvalidArgument,
		},
		{
			name: "etag mismatch",
			req: &pb.UpdateRecordRequest{
				Etag: "invalid etag",
				Record: &pb.Record{
					Name: record.FormatName(result.GetName(), "a"),
					Data: &pb.Any{
						Value: jsonutil.AnyBytes(t, tr),
					},
				},
			},
			status: codes.FailedPrecondition,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			var r *pb.Record
			if tc.record != nil {
				var err error
				r, err = srv.CreateRecord(ctx, &pb.CreateRecordRequest{
					Parent: result.GetName(),
					Record: tc.record,
				})
				if err != nil {
					t.Fatalf("CreateRecord(): %v", err)
				}
			}

			fakeClock.Advance(time.Second)

			got, err := srv.UpdateRecord(ctx, tc.req)
			// if there is an error from UpdateRecord or expecting an error here,
			// compare the two errors.
			if err != nil || tc.status != codes.OK {
				if status.Code(err) == tc.status {
					return
				}
				t.Fatalf("UpdateRecord(%+v): %v", tc.req, err)
			}

			proto.Merge(r, tc.diff)
			r.UpdatedTime = timestamppb.New(clock.Now())
			r.Etag = mockEtag(lastID, r.UpdatedTime.AsTime().UnixNano())

			if diff := cmp.Diff(r, got, protocmp.Transform()); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestDeleteRecord(t *testing.T) {
	srv, err := New(test.NewDB(t))
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

	t.Run("success", func(t *testing.T) {
		// Delete inserted record
		if _, err := srv.DeleteRecord(ctx, &pb.DeleteRecordRequest{Name: r.GetName()}); err != nil {
			t.Fatalf("could not delete record: %v", err)
		}
		// Check if the the record is deleted
		if r, err := srv.GetRecord(ctx, &pb.GetRecordRequest{Name: r.GetName()}); status.Code(err) != codes.NotFound {
			t.Fatalf("expected record to be deleted, got: %+v, %v", r, err)
		}
	})

	t.Run("already deleted", func(t *testing.T) {
		// Check if a deleted record can be deleted again
		if _, err := srv.DeleteRecord(ctx, &pb.DeleteRecordRequest{Name: r.GetName()}); status.Code(err) != codes.NotFound {
			t.Fatalf("expected NOT_FOUND, got: %v", err)
		}
	})
}

// TestListRecords_multiresult tests listing records across multiple parents.
func TestListRecords_multiresult(t *testing.T) {
	// Create a temporary database
	srv, err := New(test.NewDB(t))
	if err != nil {
		t.Fatalf("failed to setup db: %v", err)
	}
	ctx := context.Background()

	records := make([]*pb.Record, 0, 8)
	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			result, err := srv.CreateResult(ctx, &pb.CreateResultRequest{
				Parent: strconv.Itoa(i),
				Result: &pb.Result{
					Name: fmt.Sprintf("%d/results/%d", i, j),
				},
			})
			if err != nil {
				t.Fatalf("CreateResult(): %v", err)
			}
			for k := 0; k < 2; k++ {
				r, err := srv.CreateRecord(ctx, &pb.CreateRecordRequest{
					Parent: result.GetName(),
					Record: &pb.Record{
						Name: recordutil.FormatName(result.GetName(), strconv.Itoa(k)),
					},
				})
				if err != nil {
					t.Fatalf("CreateRecord(): %v", err)
				}
				records = append(records, r)
			}
		}
	}

	got, err := srv.ListRecords(ctx, &pb.ListRecordsRequest{
		Parent: "0/results/-",
	})
	if err != nil {
		t.Fatalf("ListRecords(): %v", err)
	}
	want := &pb.ListRecordsResponse{
		Records: records[:4],
	}
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Error(diff)
	}
}
