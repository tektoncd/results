package log

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/tektoncd/results/pkg/apis/v1alpha2"
	"github.com/tektoncd/results/pkg/conf"
)

const (
	// MaxLogChunkSize is the recommended maximum log chunk size.
	// This based on the recommended gRPC message size for streamed content, which ranges from 16
	// to 64 KiB. Choosing 32 KiB as a middle ground between the two.
	MaxLogChunkSize = 32 * 1024
)

type Flushable interface {
	Flush() error
}

type LogStreamer interface {
	io.ReaderFrom
	io.WriterTo
	Type() string
}

// NewLogStreamer returns a LogStreamer for the given TaskRunLog.
// LogStreamers do the following:
//
// 1. Write log data from their respective source to an io.Writer interface.
// 2. Read log data from a source, and store it in the respective backend if that behavior is supported.
//
// All LogStreamers support writing log data to an io.Writer from the provided source.
// LogStreamers do not need to receive and store data from the provided source.
//
// NewLogStreamer may mutate the TaskRunLog object's status, to provide implementation information
// for reading and writing files.
func NewLogStreamer(trl *v1alpha2.TaskRunLog, bufSize int, logDataDir string, conf *conf.ConfigFile, ctx context.Context) (LogStreamer, error) {
	switch trl.Spec.Type {
	case v1alpha2.FileLogType:
		return NewFileLogStreamer(trl, bufSize, logDataDir), nil
	case v1alpha2.S3LogType:
		return NewS3LogStreamer(trl, bufSize, conf, logDataDir, ctx)
	}
	return nil, fmt.Errorf("log streamer type %s is not supported", trl.Spec.Type)
}

func defaultFilePath(trl *v1alpha2.TaskRunLog) string {
	return filepath.Join(trl.Spec.RecordName, "taskrun.log")
}
