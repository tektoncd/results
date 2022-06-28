package server

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/tektoncd/results/pkg/api/server/test"
	recordutil "github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	"github.com/tektoncd/results/pkg/apis/v1alpha2"
	"github.com/tektoncd/results/pkg/internal/jsonutil"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"

	"google.golang.org/grpc"
)

type mockGetLogServer struct {
	grpc.ServerStream
	ctx          context.Context
	receivedData *bytes.Buffer
}

func (m *mockGetLogServer) Send(chunk *pb.LogChunk) error {
	if m.receivedData == nil {
		m.receivedData = &bytes.Buffer{}
	}
	_, err := m.receivedData.Write(chunk.GetData())
	return err
}

func (m *mockGetLogServer) Context() context.Context {
	return m.ctx
}

type mockPutLogServer struct {
	grpc.ServerStream
	ctx           context.Context
	record        *pb.Record
	logStream     []string
	bytesReceived int64
}

func (m *mockPutLogServer) Recv() (*pb.LogChunk, error) {
	if len(m.logStream) == 0 {
		return nil, io.EOF
	}
	chunk := &pb.LogChunk{
		Name: m.record.GetName(),
		Data: []byte(m.logStream[0]),
	}
	m.logStream = m.logStream[1:]
	return chunk, nil
}

func (m *mockPutLogServer) SendAndClose(s *pb.PutLogSummary) error {
	m.bytesReceived = s.BytesReceived
	return nil
}

func (m *mockPutLogServer) Context() context.Context {
	return m.ctx
}

func TestGetLog(t *testing.T) {
	srv, err := New(test.NewDB(t))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	ctx := context.Background()
	mock := &mockGetLogServer{
		ctx: ctx,
	}
	result, err := srv.CreateResult(ctx, &pb.CreateResultRequest{
		Parent: "foo",
		Result: &pb.Result{
			Name: "foo/results/bar",
		},
	})
	if err != nil {
		t.Fatalf("CreateResult: %v", err)
	}

	expectedData := "Hello World!"
	logFile, err := ioutil.TempFile("", "testgetlog-taskrun-*.log")
	t.Log("test taskrun log file: ", logFile.Name())
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

	record, err := srv.CreateRecord(ctx, &pb.CreateRecordRequest{
		Parent: result.GetName(),
		Record: &pb.Record{
			Name: recordutil.FormatName(result.GetName(), "baz-log"),
			Data: &pb.Any{
				Type: v1alpha2.TaskRunLogRecordType,
				Value: jsonutil.AnyBytes(t, &v1alpha2.TaskRunLog{
					Type: v1alpha2.FileLogType,
					File: &v1alpha2.FileLogTypeSpec{
						Path: logFile.Name(),
					},
				}),
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}

	err = srv.GetLog(&pb.GetLogRequest{
		Name: record.GetName(),
	}, mock)
	if err != nil {
		t.Errorf("failed to get log: %v", err)
	}
	actualData := mock.receivedData.String()
	if expectedData != actualData {
		t.Errorf("expected to have received %q, got %q", expectedData, actualData)
	}
}

func TestPutLog(t *testing.T) {
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

	testDir, err := ioutil.TempDir("", "testgetlog-")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}
	t.Logf("test storage directory: %s", testDir)
	t.Cleanup(func() {
		os.RemoveAll(testDir)
	})
	recordName := recordutil.FormatName(result.GetName(), "baz-log")
	path := filepath.Join(testDir, recordName, "task-run.log")
	record, err := srv.CreateRecord(ctx, &pb.CreateRecordRequest{
		Parent: result.GetName(),
		Record: &pb.Record{
			Name: recordName,
			Data: &pb.Any{
				Type: v1alpha2.TaskRunLogRecordType,
				Value: jsonutil.AnyBytes(t, &v1alpha2.TaskRunLog{
					Type: v1alpha2.FileLogType,
					File: &v1alpha2.FileLogTypeSpec{
						Path: path,
					},
				}),
			},
		},
	})
	t.Logf("Record name: %s", record.GetName())
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	mock := &mockPutLogServer{
		ctx:       ctx,
		record:    record,
		logStream: []string{"Hello world!", " This is Tekton Results."},
	}
	err = srv.PutLog(mock)
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
