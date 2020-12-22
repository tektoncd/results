package server

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/results/pkg/api/server/db/pagination"
	"github.com/tektoncd/results/pkg/api/server/internal/protoutil"
	"github.com/tektoncd/results/pkg/api/server/test"
	recordutil "github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	resultutil "github.com/tektoncd/results/pkg/api/server/v1alpha2/result"
	ppb "github.com/tektoncd/results/proto/pipeline/v1beta1/pipeline_go_proto"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestCreateRecord(t *testing.T) {
	srv, err := New(test.NewDB(t))
	if err != nil {
		t.Fatalf("failed to create temp file for db: %v", err)
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
			Data: protoutil.Any(&ppb.TaskRun{Metadata: &ppb.ObjectMeta{Name: "tacocat"}}),
		},
	}
	t.Run("success", func(t *testing.T) {
		got, err := srv.CreateRecord(ctx, req)
		if err != nil {
			t.Fatalf("CreateRecord: %v", err)
		}
		want := proto.Clone(req.GetRecord()).(*pb.Record)
		want.Id = fmt.Sprint(lastID)
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
	srv := &Server{
		db: test.NewDB(t),
		getResultID: func(context.Context, string, string) (string, error) {
			return result, nil
		},
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
		t.Fatalf("failed to create temp file for db: %v", err)
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
		r, err := srv.CreateRecord(ctx, &pb.CreateRecordRequest{
			Parent: result.GetName(),
			Record: &pb.Record{
				Name: fmt.Sprintf("%s/records/%d", result.GetName(), i),
				Data: protoutil.Any(&ppb.TaskRun{
					Metadata: &ppb.ObjectMeta{
						Name: fmt.Sprintf("%d", i),
					},
				}),
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
		r, err := srv.CreateRecord(ctx, &pb.CreateRecordRequest{
			Parent: result.GetName(),
			Record: &pb.Record{
				Name: fmt.Sprintf("%s/records/%d", result.GetName(), i),
				Data: protoutil.Any(&ppb.PipelineRun{
					Metadata: &ppb.ObjectMeta{
						Name: fmt.Sprintf("%d", i),
					},
				}),
			},
		})
		if err != nil {
			t.Fatalf("could not create result: %v", err)
		}
		t.Logf("Created record: %+v", r)
		records = append(records, r)
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
				Filter: `record.name == "foo/results/bar/records/0"`,
			},
			want: &pb.ListRecordsResponse{
				Records: records[:1],
			},
		},
		{
			name: "filter by record data",
			req: &pb.ListRecordsRequest{
				Parent: result.GetName(),
				Filter: `record.data.metadata.name == "0"`,
			},
			want: &pb.ListRecordsResponse{
				Records: records[:1],
			},
		},
		{
			name: "filter by record type",
			req: &pb.ListRecordsRequest{
				Parent: result.GetName(),
				Filter: `type(record.data) == tekton.pipeline.v1beta1.TaskRun`,
			},
			want: &pb.ListRecordsResponse{
				Records: records[:3],
			},
		},
		{
			name: "filter by parent",
			req: &pb.ListRecordsRequest{
				Parent: result.GetName(),
				Filter: fmt.Sprintf(`record.name.startsWith("%s")`, result.GetName()),
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
				Filter:   "type(record.data) == tekton.pipeline.v1beta1.TaskRun",
				PageSize: 1,
			},
			want: &pb.ListRecordsResponse{
				Records:       records[:1],
				NextPageToken: pagetoken(t, records[1].GetId(), "type(record.data) == tekton.pipeline.v1beta1.TaskRun"),
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
