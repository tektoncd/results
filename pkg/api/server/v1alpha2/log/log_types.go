package log

import (
	"fmt"
	"io"
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

func NewLogStreamer(trl *TaskRunLog) (LogStreamer, error) {
	switch trl.Type {
	case FileLogType:
		return NewFileLogStreamer(trl)
	}
	return nil, fmt.Errorf("log streamer type %s is not supported", trl.Type)
}
