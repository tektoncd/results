package logs

import (
	"bytes"
	"testing"

	"google.golang.org/genproto/googleapis/api/httpbody"

	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

type mockGetLogServer struct {
	receivedData *bytes.Buffer
}

func (m *mockGetLogServer) Send(log *pb.Log) error {
	if m.receivedData == nil {
		m.receivedData = &bytes.Buffer{}
	}
	_, err := m.receivedData.Write(log.GetData())
	return err
}

type mockGetLogHTTPServer struct {
	receivedData *bytes.Buffer
}

func (m *mockGetLogHTTPServer) Send(log *httpbody.HttpBody) error {
	if m.receivedData == nil {
		m.receivedData = &bytes.Buffer{}
	}
	_, err := m.receivedData.Write(log.GetData())
	return err
}

func TestBufferedLog_Write(t *testing.T) {
	sender := &mockGetLogServer{
		receivedData: &bytes.Buffer{},
	}

	httpSender := &mockGetLogHTTPServer{
		receivedData: &bytes.Buffer{},
	}

	data := "Hello!"
	size := 10
	writer := NewBufferedWriter(sender, "test-result", size)
	bytes, err := writer.Write([]byte(data))
	if err != nil {
		t.Errorf("failed to write bytes: %v", err)
	}
	if bytes != len(data) {
		t.Errorf("writer should record %d bytes, but recorded %d", len(data), bytes)
	}

	writer = NewBufferedHTTPWriter(httpSender, "test-result", size)
	bytes, err = writer.Write([]byte(data))
	if err != nil {
		t.Errorf("failed to write bytes: %v", err)
	}
	if bytes != len(data) {
		t.Errorf("writer should record %d bytes, but recorded %d", len(data), bytes)
	}
}

func TestBufferedLog_Flush(t *testing.T) {
	tests := []struct {
		name string
		data string
		size int
	}{
		{
			name: "when content is longer than chunk",
			data: "Testing!",
			size: 5,
		},
		{
			name: "when content is smaller than chunk",
			data: "Testing!",
			size: 10,
		},
		{
			name: "when content is equal to chunk",
			data: "Testing!",
			size: 8,
		},
		{
			name: "when content is equal to few chunks",
			data: "Testing!",
			size: 4,
		},
		{
			name: "when content is more than few chunks",
			data: "Testing!",
			size: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sender := &mockGetLogServer{
				receivedData: &bytes.Buffer{},
			}

			httpSender := &mockGetLogHTTPServer{
				receivedData: &bytes.Buffer{},
			}

			writer := NewBufferedWriter(sender, "test-result", tc.size)
			_, err := writer.Write([]byte(tc.data))
			if err != nil {
				t.Errorf("failed to write bytes: %v", err)
			}

			_, err = writer.Flush()
			if err != nil {
				t.Error("failed to write remaining bytes")
			}

			received := sender.receivedData.String()
			if received != tc.data {
				t.Errorf("expected to receive %q, but received %q", tc.data, received)
			}

			writer = NewBufferedHTTPWriter(httpSender, "test-result", tc.size)
			_, err = writer.Write([]byte(tc.data))
			if err != nil {
				t.Errorf("failed to write bytes: %v", err)
			}

			_, err = writer.Flush()
			if err != nil {
				t.Error("failed to write remaining bytes")
			}

			received = sender.receivedData.String()
			if received != tc.data {
				t.Errorf("expected to receive %q, but received %q", tc.data, received)
			}
		})
	}
}
