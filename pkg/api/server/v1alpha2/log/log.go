package log

import (
	"fmt"
	"io"

	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

const (
	// MaxLogChunkSize is the recommended maximum log chunk size.
	// This based on the recommended gRPC message size for streamed content, which ranges from 16
	// to 64 KiB. Choosing 32 KiB as a middle ground between the two.
	MaxLogChunkSize = 32 * 1024
)

type LogStreamer interface {
	io.WriterTo
	Type() string
}

type TaskRunLogType string

const (
	FileLogType TaskRunLogType = "File"
)

type TaskRunLog struct {
	Type TaskRunLogType   `json:"type"`
	File *FileLogTypeSpec `json:"file,omitempty"`
}

type FileLogTypeSpec struct {
	Path string `json:"path"`
}

// NewLogStreamer returns a LogStreamer for the given TaskRunLog.
// LogStreamers write log data from their respective source to an io.Writer interface.
func NewLogStreamer(trl *TaskRunLog, bufSize int) (LogStreamer, error) {
	switch trl.Type {
	case FileLogType:
		return NewFileLogStreamer(trl, bufSize)
	}
	return nil, fmt.Errorf("log streamer type %s is not supported", trl.Type)
}

type logChunkWriter struct {
	server       pb.Results_GetLogServer
	maxChunkSize int
}

// NewLogChunkWriter returns an io.Writer that writes log chunk messages over gRPC.
func NewLogChunkWriter(srv pb.Results_GetLogServer, chunkSize int) io.Writer {
	return &logChunkWriter{
		server:       srv,
		maxChunkSize: chunkSize,
	}
}

// Write sends the provided bytes over gRPC. If the length of the byte array exceeds the maximum
// log chunk size, the data is sent in multiple chunks.
func (w *logChunkWriter) Write(p []byte) (int, error) {
	// If the array length is less than or equal to the MaxLogChunkSize, send the entire byte array
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
func (w *logChunkWriter) sendBytes(p []byte) (int, error) {
	logChunk := &pb.LogChunk{Data: p}
	err := w.server.Send(logChunk)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}
