package log

import (
	"fmt"
	"io"

	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

const (
	// DefaultMaxLogChunkSize is the default maximum log chunk size. This based on the recommended
	// gRPC message size for streamed content, which ranges from 16 to 64 KiB. Choosing 32 KiB as a
	// middle ground between the two.
	DefaultMaxLogChunkSize = 32 * 1024
)

type Sender interface {
	Send(log *pb.Log) error
}

type log struct {
	sender       Sender
	recordName   string
	maxChunkSize int
}

// NewLogWriter returns an io.Writer that writes log chunk messages to the gRPC sender for the
// named Tekton result. The chunk size determines the maximum size of a single sent message - if
// less than zero, this defaults to DefaultMaxLogChunkSize.
func NewLogWriter(sender Sender, recordName string, chunkSize int) io.Writer {
	if chunkSize < 1 {
		chunkSize = DefaultMaxLogChunkSize
	}
	return &log{
		sender:       sender,
		recordName:   recordName,
		maxChunkSize: chunkSize,
	}
}

// Write sends the provided bytes over gRPC. If the length of the byte array exceeds the maximum
// log chunk size, the data is sent in multiple chunks.
func (w *log) Write(p []byte) (int, error) {
	// If the array length is less than or equal to the MaxLogChunkSize, send the entire byte array
	fmt.Printf("writing %d bytes for %s\n", len(p), w.recordName)
	if len(p) <= w.maxChunkSize {
		return w.sendBytes(p)
	}
	// Send the slice, up to MaxLogChunkSize
	sent, err := w.sendBytes(p[:w.maxChunkSize])
	if err != nil {
		return sent, err
	}
	nextWrites, err := w.Write(p[w.maxChunkSize:])
	sent += nextWrites
	return sent, err
}

// sendBytes sends the provided byte array over gRPC.
func (w *log) sendBytes(p []byte) (int, error) {
	log := &pb.Log{
		Name: w.recordName,
		Data: p,
	}
	err := w.sender.Send(log)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}
