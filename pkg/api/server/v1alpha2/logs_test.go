package server

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/tektoncd/results/pkg/api/server/test"
	recordutil "github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
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
