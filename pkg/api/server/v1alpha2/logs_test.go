package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/api/httpbody"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/results/pkg/api/server/config"
	"github.com/tektoncd/results/pkg/api/server/db/pagination"
	"github.com/tektoncd/results/pkg/api/server/logger"
	"github.com/tektoncd/results/pkg/api/server/test"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/log"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/result"
	"github.com/tektoncd/results/pkg/apis/v1alpha2"
	"github.com/tektoncd/results/pkg/internal/jsonutil"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"google.golang.org/grpc"
)

type mockGetLogServer struct {
	grpc.ServerStream
	ctx          context.Context
	receivedData *bytes.Buffer
}

func (m *mockGetLogServer) Send(chunk *httpbody.HttpBody) error {
	if m.receivedData == nil {
		m.receivedData = &bytes.Buffer{}
	}
	_, err := m.receivedData.Write(chunk.GetData())
	return err
}

func (m *mockGetLogServer) Context() context.Context {
	return m.ctx
}

type mockUpdateLogServer struct {
	grpc.ServerStream
	ctx           context.Context
	record        *pb.Record
	logStream     []string
	bytesReceived int64
}

func (m *mockUpdateLogServer) Recv() (*pb.Log, error) {
	if len(m.logStream) == 0 {
		return nil, io.EOF
	}

	parent, resultName, recordName, err := record.ParseName(m.record.GetName())
	if err != nil {
		return nil, err
	}
	chunk := &pb.Log{
		Name: log.FormatName(result.FormatName(parent, resultName), recordName),
		Data: []byte(m.logStream[0]),
	}
	m.logStream = m.logStream[1:]
	return chunk, nil
}

func (m *mockUpdateLogServer) SendAndClose(s *pb.LogSummary) error {
	m.bytesReceived = s.BytesReceived
	return nil
}

func (m *mockUpdateLogServer) Context() context.Context {
	return m.ctx
}

func TestGetLog(t *testing.T) {
	srv, err := New(&config.Config{
		LOGS_API:                 true,
		LOGS_TYPE:                "File",
		DB_ENABLE_AUTO_MIGRATION: true,
	}, logger.Get("info"), test.NewDB(t))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	ctx := context.Background()
	mock := &mockGetLogServer{
		ctx: ctx,
	}
	res, err := srv.CreateResult(ctx, &pb.CreateResultRequest{
		Parent: "foo",
		Result: &pb.Result{
			Name: "foo/results/bar",
		},
	})
	if err != nil {
		t.Fatalf("CreateResult: %v", err)
	}

	expectedData := "Hello World!"
	logFile, err := os.CreateTemp("", "test-log-taskrun-*.log")
	t.Log("test log file: ", logFile.Name())
	t.Cleanup(func() {
		logFile.Close()
		os.Remove(logFile.Name())
	})
	if err != nil {
		t.Fatalf("failed to create tempfile: %v", err)
	}
	_, err = logFile.Write([]byte(expectedData))
	if err != nil {
		t.Fatalf("failed to write to tempfile: %v", err)
	}

	_, err = srv.CreateRecord(ctx, &pb.CreateRecordRequest{
		Parent: res.GetName(),
		Record: &pb.Record{
			Name: record.FormatName(res.GetName(), "baz"),
			Data: &pb.Any{
				Type: v1alpha2.LogRecordType,
				Value: jsonutil.AnyBytes(t, &v1alpha2.Log{
					Spec: v1alpha2.LogSpec{
						Resource: v1alpha2.Resource{
							Namespace: "foo",
							Name:      "baz",
						},
						Type: v1alpha2.FileLogType,
					},
					// To avoid defaulting behavior, explicitly set the file path in status
					Status: v1alpha2.LogStatus{
						Path: logFile.Name(),
						Size: 1024,
					},
				}),
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}

	err = srv.GetLog(&pb.GetLogRequest{
		Name: log.FormatName(res.GetName(), "baz"),
	}, mock)
	if err != nil {
		t.Fatalf("failed to get log: %v", err)
	}
	actualData := mock.receivedData.String()
	if expectedData != actualData {
		t.Errorf("expected to have received %q, got %q", expectedData, actualData)
	}
}

func TestUpdateLog(t *testing.T) {
	testDir, err := os.MkdirTemp("", "test-logs-")
	if err != nil {
		t.Fatalf("failed to test temp folder: %v", err)
	}
	c := &config.Config{
		LOGS_TYPE:                "File",
		LOGS_API:                 true,
		DB_ENABLE_AUTO_MIGRATION: true,
	}
	srv, err := New(c, logger.Get("info"), test.NewDB(t))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	ctx := context.Background()
	res, err := srv.CreateResult(ctx, &pb.CreateResultRequest{
		Parent: "foo",
		Result: &pb.Result{
			Name: "foo/results/bar",
		},
	})
	if err != nil {
		t.Fatalf("CreateResult: %v", err)
	}
	t.Logf("test storage directory: %s", testDir)
	t.Cleanup(func() {
		os.RemoveAll(testDir)
	})
	recordName := record.FormatName(res.GetName(), "baz-log")
	path := filepath.Join(testDir, "test-uid", "task-run.log")
	rec, err := srv.CreateRecord(ctx, &pb.CreateRecordRequest{
		Parent: res.GetName(),
		Record: &pb.Record{
			Name: recordName,
			Data: &pb.Any{
				Type: v1alpha2.LogRecordType,
				Value: jsonutil.AnyBytes(t, &v1alpha2.Log{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-name",
						UID:  "test-uid",
					},
					Spec: v1alpha2.LogSpec{
						Resource: v1alpha2.Resource{
							Namespace: "foo",
							Name:      "baz",
						},
						Type: v1alpha2.FileLogType,
					},
					// To avoid defaulting behavior, explicitly set the file path in status
					Status: v1alpha2.LogStatus{
						Path: path,
					},
				}),
			},
		},
	})
	t.Logf("Record name: %s", rec.GetName())
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}

	mock := &mockUpdateLogServer{
		ctx:       ctx,
		record:    rec,
		logStream: []string{"Hello world! ", "This is Tekton Results."},
	}
	err = srv.UpdateLog(mock)
	if err != nil {
		t.Fatalf("failed to put log: %v", err)
	}
	actualData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read data from file: %v", err)
	}
	expectedData := "Hello world! This is Tekton Results."
	if expectedData != string(actualData) {
		t.Errorf("expected to have received %q, got %q", expectedData, actualData)
	}
	if mock.bytesReceived != int64(len(expectedData)) {
		t.Errorf("expected to have received %d bytes, got %d", len(expectedData), mock.bytesReceived)
	}
}

func TestListLogs(t *testing.T) {
	// Create a temporary database
	srv, err := New(&config.Config{
		LOGS_API:                 true,
		LOGS_TYPE:                "File",
		DB_ENABLE_AUTO_MIGRATION: true,
	}, logger.Get("info"), test.NewDB(t))
	if err != nil {
		t.Fatalf("failed to setup db: %v", err)
	}
	ctx := context.Background()

	res, err := srv.CreateResult(ctx, &pb.CreateResultRequest{
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
			Parent: res.GetName(),
			Record: &pb.Record{
				Name: fmt.Sprintf("%s/records/%d", res.GetName(), i),
				Data: &pb.Any{
					Type: v1alpha2.LogRecordType,
					Value: jsonutil.AnyBytes(t, &v1alpha2.Log{
						ObjectMeta: metav1.ObjectMeta{
							Name: fmt.Sprintf("%d", i),
						},
					}),
				},
			},
		})
		if err != nil {
			t.Fatalf("could not create record: %v", err)
		}
		t.Logf("Created record: %+v", r)
		r.Name = log.FormatName(res.GetName(), strconv.Itoa(i))
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
				Parent: res.GetName(),
			},
			want: &pb.ListRecordsResponse{
				Records: records,
			},
		},
		{
			name: "list all records without knowing the result name",
			req: &pb.ListRecordsRequest{
				Parent: "foo/results/-",
			},
			want: &pb.ListRecordsResponse{
				Records: records,
			},
		},
		{
			name: "list all records without knowing the parent and the result name",
			req: &pb.ListRecordsRequest{
				Parent: "-/results/-",
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
				Parent: res.GetName(),
				Filter: `name == "foo/results/bar/records/0"`,
			},
			want: &pb.ListRecordsResponse{
				Records: records[:1],
			},
		},
		{
			name: "filter by record data",
			req: &pb.ListRecordsRequest{
				Parent: res.GetName(),
				Filter: `data.metadata.name == "0"`,
			},
			want: &pb.ListRecordsResponse{
				Records: records[:1],
			},
		},
		{
			name: "filter by parent",
			req: &pb.ListRecordsRequest{
				Parent: res.GetName(),
				Filter: fmt.Sprintf(`name.startsWith("%s")`, res.GetName()),
			},
			want: &pb.ListRecordsResponse{
				Records: records,
			},
		},
		// Pagination
		{
			name: "filter and page size",
			req: &pb.ListRecordsRequest{
				Parent:   res.GetName(),
				Filter:   `data_type == "results.tekton.dev/v1alpha2.Log"`,
				PageSize: 1,
			},
			want: &pb.ListRecordsResponse{
				Records:       records[:1],
				NextPageToken: pagetoken(t, records[1].GetUid(), `data_type == "results.tekton.dev/v1alpha2.Log"`),
			},
		},
		{
			name: "only page size",
			req: &pb.ListRecordsRequest{
				Parent:   res.GetName(),
				PageSize: 1,
			},
			want: &pb.ListRecordsResponse{
				Records:       records[:1],
				NextPageToken: pagetoken(t, records[1].GetUid(), ""),
			},
		},
		// Order By
		{
			name: "with order asc",
			req: &pb.ListRecordsRequest{
				Parent:  res.GetName(),
				OrderBy: "created_time asc",
			},
			want: &pb.ListRecordsResponse{
				Records: records,
			},
		},
		{
			name: "with order desc",
			req: &pb.ListRecordsRequest{
				Parent:  res.GetName(),
				OrderBy: "created_time desc",
			},
			want: &pb.ListRecordsResponse{
				Records: reversedRecords,
			},
		},
		{
			name: "with missing order",
			req: &pb.ListRecordsRequest{
				Parent:  res.GetName(),
				OrderBy: "",
			},
			want: &pb.ListRecordsResponse{
				Records: records,
			},
		},
		{
			name: "with default order",
			req: &pb.ListRecordsRequest{
				Parent:  res.GetName(),
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
				Parent: res.GetName(),
				Filter: `type(record.data) == tekton.pipeline.v1beta1.Unknown`,
			},
			status: codes.InvalidArgument,
		},
		{
			name: "unknown any field",
			req: &pb.ListRecordsRequest{
				Parent: res.GetName(),
				Filter: `record.data.metadata.unknown == "tacocat"`,
			},
			status: codes.InvalidArgument,
		},
		{
			name: "invalid page size",
			req: &pb.ListRecordsRequest{
				Parent:   res.GetName(),
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
				Parent:  res.GetName(),
				OrderBy: "created_time desc asc",
			},
			status: codes.InvalidArgument,
		},
		{
			name: "invalid sort direction",
			req: &pb.ListRecordsRequest{
				Parent:  res.GetName(),
				OrderBy: "created_time foo",
			},
			status: codes.InvalidArgument,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			got, err := srv.ListLogs(ctx, tc.req)
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

// TestListRecords_multiresult tests listing records across multiple parents.
func TestListLogs_multiresult(t *testing.T) {
	// Create a temporary database
	srv, err := New(&config.Config{
		LOGS_API:                 true,
		LOGS_TYPE:                "File",
		DB_ENABLE_AUTO_MIGRATION: true,
	}, logger.Get("info"), test.NewDB(t))
	if err != nil {
		t.Fatalf("failed to setup db: %v", err)
	}
	ctx := context.Background()

	records := make([]*pb.Record, 0, 8)
	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			res, err := srv.CreateResult(ctx, &pb.CreateResultRequest{
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
					Parent: res.GetName(),
					Record: &pb.Record{
						Name: record.FormatName(res.GetName(), strconv.Itoa(k)),
						Data: &pb.Any{
							Type: v1alpha2.LogRecordType,
							Value: jsonutil.AnyBytes(t, &v1alpha2.Log{
								ObjectMeta: metav1.ObjectMeta{
									Name: fmt.Sprintf("%d", k),
								},
							}),
						},
					},
				})
				if err != nil {
					t.Fatalf("CreateRecord(): %v", err)
				}
				r.Name = log.FormatName(res.GetName(), strconv.Itoa(k))
				records = append(records, r)
			}
		}
	}

	got, err := srv.ListLogs(ctx, &pb.ListRecordsRequest{
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

func TestDeleteLog(t *testing.T) {
	srv, err := New(&config.Config{
		LOGS_API:                 true,
		LOGS_TYPE:                "File",
		DB_ENABLE_AUTO_MIGRATION: true,
	}, logger.Get("info"), test.NewDB(t))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ctx := context.Background()

	// Create result
	res, err := srv.CreateResult(ctx, &pb.CreateResultRequest{
		Parent: "foo",
		Result: &pb.Result{
			Name: "foo/results/bar",
		},
	})
	if err != nil {
		t.Fatalf("CreateResult: %v", err)
	}

	// Create log to stream
	logFile, err := os.CreateTemp("", "test-log-taskrun-*.log")
	t.Log("test log file: ", logFile.Name())
	t.Cleanup(func() {
		logFile.Close()
		os.Remove(logFile.Name())
	})
	if err != nil {
		t.Fatalf("failed to create tempfile: %v", err)
	}
	_, err = logFile.Write([]byte("test data"))
	if err != nil {
		t.Fatalf("failed to write to tempfile: %v", err)
	}

	// Create record
	rec, err := srv.CreateRecord(ctx, &pb.CreateRecordRequest{
		Parent: res.GetName(),
		Record: &pb.Record{
			Name: record.FormatName(res.GetName(), "baz"),
			Data: &pb.Any{
				Type: v1alpha2.LogRecordType,
				Value: jsonutil.AnyBytes(t, &v1alpha2.Log{
					Spec: v1alpha2.LogSpec{
						Resource: v1alpha2.Resource{
							Namespace: "foo",
							Name:      "baz",
						},
						Type: v1alpha2.FileLogType,
					},
					// To avoid defaulting behavior, explicitly set the file path in status
					Status: v1alpha2.LogStatus{
						Path: logFile.Name(),
						Size: 1024,
					},
				}),
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateRecord(): %v", err)
	}
	logName := log.FormatName(res.GetName(), "baz")
	t.Run("success", func(t *testing.T) {
		// Delete inserted record
		if _, err := srv.DeleteLog(ctx, &pb.DeleteLogRequest{Name: logName}); err != nil {
			t.Fatalf("could not delete record: %v", err)
		}
		// Check if the record is deleted
		if r, err := srv.GetRecord(ctx, &pb.GetRecordRequest{Name: rec.GetName()}); status.Code(err) != codes.NotFound {
			t.Fatalf("expected record to be deleted, got: %+v, %v", r, err)
		}
		// Check if the file is deleted
		if _, err := os.Stat(logFile.Name()); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("could not delete log file: %v", err)
		}
	})

	t.Run("already deleted", func(t *testing.T) {
		// Check if a deleted record can be deleted again
		if _, err := srv.DeleteLog(ctx, &pb.DeleteLogRequest{Name: logName}); status.Code(err) != codes.NotFound {
			t.Fatalf("expected NOT_FOUND, got: %v", err)
		}
	})
}
