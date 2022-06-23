package log

import (
	"bufio"
	"fmt"
	"io"
	"os"
)

type fileLogStreamer struct {
	path string
}

// NewFileLogStreamer returns a LogStreamer that streams directly from a log file on local disk.
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

func (f *fileLogStreamer) WriteTo(w io.Writer) (n int64, err error) {
	_, err = os.Stat(f.path)
	if err != nil {
		return 0, fmt.Errorf("failed to stat %s: %v", f.path, err)
	}
	file, err := os.Open(f.path)
	if err != nil {
		return 0, fmt.Errorf("failed to open file %s: %v", f.path, err)
	}
	defer func() {
		closeErr := file.Close()
		if err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	// Use the bufferred reader to ensure file contents are not read entirely into memory
	reader := bufio.NewReader(file)
	n, err = reader.WriteTo(w)
	return
}
