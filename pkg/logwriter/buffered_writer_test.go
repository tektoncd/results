package log

import (
	"bytes"
	"testing"

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

// Test case: "Hello!" has got 6 bytes. Chunks size is 5. Remain is 6 - 5 = 1.
// NewBufferedLogWriter.Write([]byte) should send 5 bytes(1 chunk) to sender
// and save 1 byte in the buffer.
// NewBufferedLogWriter.WriteRemain() should send last byte to sender.
func TestBufferedLogWriterWhenContentIsLongerThenChunk(t *testing.T) {
	sender := &mockGetLogServer{
		receivedData: &bytes.Buffer{},
	}

	expected := "Hello!"
	chunkSize := 5
	writer := NewBufferedLogWriter(sender, "test-result", chunkSize)

	remainBytes := len(expected) - chunkSize

	readBts, err := writer.Write([]byte(expected))
	if err != nil {
		t.Errorf("failed to write bytes: %v", err)
	}
	if readBts != len(expected) {
		t.Errorf("writer should record %d bytes, but recorded %d", len(expected), readBts)
	}

	expectedBts := []byte(expected)
	expectedBts = expectedBts[:chunkSize]
	received := sender.receivedData.String()
	if received != string(expectedBts) {
		t.Errorf("expected to receive %q, got %q", expected, received)
	}

	n, err := writer.WriteRemain()
	if err != nil {
		t.Error("failed to write remaining bytes")
	}

	if n != remainBytes {
		t.Error("writer should write last remaining byte")
	}

	received = sender.receivedData.String()
	if received != expected {
		t.Errorf("expected to receive %q, got %q", expected, received)
	}
}

// Test case: "Hello!" has got 6 bytes. Chunks size is 7.
// NewBufferedLogWriter.Write([]byte) should not send 7 bytes to sender
// and save them in the buffer.
// NewBufferedLogWriter.WriteRemain() should send 7 bytes to sender.
func TestBufferedLogWriterWhenContentIsSmallerThenChunk(t *testing.T) {
	sender := &mockGetLogServer{
		receivedData: &bytes.Buffer{},
	}

	expected := "Hello!"
	chunkSize := 7
	writer := NewBufferedLogWriter(sender, "test-result", chunkSize)

	remainBytes := len(expected)

	readBts, err := writer.Write([]byte(expected))
	if err != nil {
		t.Errorf("failed to write bytes: %v", err)
	}
	if readBts != len(expected) {
		t.Errorf("writer should record %d bytes, but recorded %d", len(expected), readBts)
	}

	received := sender.receivedData.String()
	if received != "" {
		t.Error("expected none received bytes")
	}

	n, err := writer.WriteRemain()
	if err != nil {
		t.Error("failed to write remaining bytes")
	}

	if n != remainBytes {
		t.Errorf("writer should write all %d buffered bytes", remainBytes)
	}

	received = sender.receivedData.String()
	if received != expected {
		t.Errorf("expected to receive %q, got %q", expected, received)
	}
}

// Test case: "Hello!" has got 6 bytes. Chunks size is 6.
// NewBufferedLogWriter.Write([]byte) should send 6 bytes to sender
// and doesn't save any bytes in the buffer.
// NewBufferedLogWriter.WriteRemain() should send 0 bytes to sender.
func TestBufferedLogWriterWhenContentIsEqualThenChunk(t *testing.T) {
	sender := &mockGetLogServer{
		receivedData: &bytes.Buffer{},
	}

	expected := "Hello!"
	chunkSize := 6
	writer := NewBufferedLogWriter(sender, "test-result", chunkSize)

	remainBytes := 0

	readBts, err := writer.Write([]byte(expected))
	if err != nil {
		t.Errorf("failed to write bytes: %v", err)
	}
	if readBts != len(expected) {
		t.Errorf("writer should record %d bytes, but recorded %d", len(expected), readBts)
	}

	expectedBts := []byte(expected)
	expectedBts = expectedBts[:chunkSize]
	received := sender.receivedData.String()
	if received != string(expectedBts) {
		t.Errorf("expected to receive %q, got %q", expected, received)
	}

	n, err := writer.WriteRemain()
	if err != nil {
		t.Error("failed to write remaining bytes")
	}

	if n != remainBytes {
		t.Error("writer should write none remaining bytes")
	}

	received = sender.receivedData.String()
	if received != expected {
		t.Errorf("expected to receive %q, got %q", expected, received)
	}
}

// Test case: "Hello!" has got 6 bytes. Chunks size is 3.
// NewBufferedLogWriter.Write([]byte) should send 6 bytes(2 chunks) to sender
// and doesn't save any bytes in the buffer.
// NewBufferedLogWriter.WriteRemain() should send 0 bytes to sender.
func TestBufferedLogWriterWhenContentIsEqualToFewChunks(t *testing.T) {
	sender := &mockGetLogServer{
		receivedData: &bytes.Buffer{},
	}

	expected := "Hello!"
	chunkSize := 3
	writer := NewBufferedLogWriter(sender, "test-result", chunkSize)

	remainBytes := 0

	readBts, err := writer.Write([]byte(expected))
	if err != nil {
		t.Errorf("failed to write bytes: %v", err)
	}
	if readBts != len(expected) {
		t.Errorf("writer should record %d bytes, but recorded %d", len(expected), readBts)
	}

	expectedBts := []byte(expected)
	expectedBts = expectedBts[:chunkSize*2]
	received := sender.receivedData.String()
	if received != string(expectedBts) {
		t.Errorf("expected to receive %q, got %q", expected, received)
	}

	n, err := writer.WriteRemain()
	if err != nil {
		t.Error("failed to write remaining bytes")
	}

	if n != remainBytes {
		t.Error("writer should write none remaining bytes")
	}

	received = sender.receivedData.String()
	if received != expected {
		t.Errorf("expected to receive %q, got %q", expected, received)
	}
}

// Test case: "Hello !" has got 7 bytes. Chunks size is 3.
// NewBufferedLogWriter.Write([]byte) should send 6 bytes(2 chunks) to sender
// and save 1 last byte in the buffer.
// NewBufferedLogWriter.WriteRemain() should send 1 byte to sender.
func TestBufferedLogWriterWhenContentWithMoreThenFewChunks(t *testing.T) {
	sender := &mockGetLogServer{
		receivedData: &bytes.Buffer{},
	}

	expected := "Hello !"
	chunkSize := 3
	writer := NewBufferedLogWriter(sender, "test-result", chunkSize)

	remainBytes := 1

	readBts, err := writer.Write([]byte(expected))
	if err != nil {
		t.Errorf("failed to write bytes: %v", err)
	}
	if readBts != len(expected) {
		t.Errorf("writer should record %d bytes, but recorded %d", len(expected), readBts)
	}

	expectedBts := []byte(expected)
	expectedBts = expectedBts[:chunkSize*2]
	received := sender.receivedData.String()
	if received != string(expectedBts) {
		t.Errorf("expected to receive %q, got %q", expected, received)
	}

	n, err := writer.WriteRemain()
	if err != nil {
		t.Error("failed to write remaining bytes")
	}

	if n != remainBytes {
		t.Error("writer should write last remaining byte")
	}

	received = sender.receivedData.String()
	if received != expected {
		t.Errorf("expected to receive %q, got %q", expected, received)
	}
}
