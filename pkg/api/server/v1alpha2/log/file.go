package log

import (
	"fmt"
	"io"
)

type fileLogStreamer struct {
	path string
}

func NewFileLogStreamer(trl *TaskRunLog) (LogStreamer, error) {
	if trl.File == nil {
		return nil, fmt.Errorf("file to stream is not specified")
	}
	return &fileLogStreamer{
		path: trl.File.Path,
	}, nil
}

func (*fileLogStreamer) Type() string {
	return string(FileLogType)
}

func (f *fileLogStreamer) WriteTo(w io.Writer) (int64, error) {
	return 0, nil
}
