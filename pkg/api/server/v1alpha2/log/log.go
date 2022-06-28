package log

import (
	"fmt"
	"io"
)

const (
	// MaxLogChunkSize is the recommended maximum log chunk size.
	// This based on the recommended gRPC message size for streamed content, which ranges from 16
	// to 64 KiB. Choosing 32 KiB as a middle ground between the two.
	MaxLogChunkSize = 32 * 1024
)

type LogStreamer interface {
	io.ReaderFrom
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
// LogStreamers do the following:
//
// 1. Write log data from their respective source to an io.Writer interface.
// 2. Read log data from a source, and store it in the respective backend if that behavior is supported.
//
// All LogStreamers support writing log data to an io.Writer from the provided source.
// LogStreamers do not need to receive and store data from the provided source.
func NewLogStreamer(trl *TaskRunLog, bufSize int) (LogStreamer, error) {
	switch trl.Type {
	case FileLogType:
		return NewFileLogStreamer(trl, bufSize)
	}
	return nil, fmt.Errorf("log streamer type %s is not supported", trl.Type)
}
