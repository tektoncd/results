package log

import (
	"bytes"
	"testing"

	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc"
)

type mockGetLogServer struct {
	grpc.ServerStream
	receivedData *bytes.Buffer
}

func (m *mockGetLogServer) Send(chunk *pb.LogChunk) error {
	if m.receivedData == nil {
		m.receivedData = &bytes.Buffer{}
	}
	_, err := m.receivedData.Write(chunk.GetData())
	return err
}

func TestLogChunkWriter(t *testing.T) {
	server := &mockGetLogServer{
		receivedData: &bytes.Buffer{},
	}
	// Test with a very low chunk size, to ensure we test recursion
	writer := NewLogChunkWriter(server, 4)
	expected := "Hello World! This is a log message!"
	n, err := writer.Write([]byte(expected))
	if err != nil {
		t.Errorf("failed to write bytes: %v", err)
	}
	if n != len(expected) {
		t.Errorf("expected %d bytes to be written, got %d", len(expected), n)
	}
	received := server.receivedData.String()
	if received != expected {
		t.Errorf("expected to receive %q, got %q", expected, received)
	}
}
