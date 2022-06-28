package log

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tektoncd/results/pkg/apis/v1alpha2"
)

type fileLogStreamer struct {
	path    string
	bufSize int
}

// NewFileLogStreamer returns a LogStreamer that streams directly from a log file on local disk.
func NewFileLogStreamer(trl *v1alpha2.TaskRunLog, bufSize int) (LogStreamer, error) {
	if trl.File == nil {
		return nil, fmt.Errorf("file to stream is not specified")
	}
	return &fileLogStreamer{
		path:    trl.File.Path,
		bufSize: bufSize,
	}, nil
}

func (*fileLogStreamer) Type() string {
	return string(v1alpha2.FileLogType)
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
	reader := bufio.NewReaderSize(file, f.bufSize)
	n, err = reader.WriteTo(w)
	return
}

func (f *fileLogStreamer) ReadFrom(r io.Reader) (n int64, err error) {
	// Ensure that the directories in the path already exist
	dir := filepath.Dir(f.path)
	err = os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return 0, fmt.Errorf("failed to create directory %s, %v", dir, err)
	}
	// Open the file with Append + Create + WriteOnly modes.
	// This ensures the file is created if it does not exist.
	// If the file does exist, data is appended instead of overwritten/truncated
	file, err := os.OpenFile(f.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, fmt.Errorf("failed to open file %s: %v", f.path, err)
	}
	defer func() {
		closeErr := file.Close()
		if err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	writer := bufio.NewWriterSize(file, f.bufSize)
	n, err = writer.ReadFrom(r)
	if err != nil {
		return
	}
	err = writer.Flush()
	return
}
