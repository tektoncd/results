package log

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tektoncd/results/pkg/api/server/config"

	"github.com/tektoncd/results/pkg/apis/v1alpha2"
)

type fileStream struct {
	path string
	size int
	ctx  context.Context
}

// NewFileStream returns a LogStreamer that streams directly from a log file on local disk.
func NewFileStream(ctx context.Context, log *v1alpha2.Log, config *config.Config) (Stream, error) {
	if log.Status.Path == "" {
		filePath, err := FilePath(log)
		if err != nil {
			return nil, err
		}
		log.Status.Path = filePath
	}

	size := config.LOGS_BUFFER_SIZE
	if size < 1 {
		size = DefaultBufferSize
	}

	return &fileStream{
		path: filepath.Join(config.LOGS_PATH, log.Status.Path),
		size: size,
		ctx:  ctx,
	}, nil
}

func (*fileStream) Type() string {
	return string(v1alpha2.FileLogType)
}

// WriteTo reads the contents of the TaskRun log file and writes them to the provided writer, such
// as os.Stdout.
func (fs *fileStream) WriteTo(w io.Writer) (n int64, err error) {
	_, err = os.Stat(fs.path)
	if err != nil {
		return 0, fmt.Errorf("failed to stat %s: %w", fs.path, err)
	}
	file, err := os.Open(fs.path)
	if err != nil {
		return 0, fmt.Errorf("failed to open file %s: %w", fs.path, err)
	}
	defer func() {
		closeErr := file.Close()
		if err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	// Use the buffered reader to ensure file contents are not read entirely into memory
	reader := bufio.NewReaderSize(file, fs.size)
	n, err = reader.WriteTo(w)
	return
}

// ReadFrom reads the log contents from the provided io.Reader, and writes them to the TaskRun log
// file on disk.
func (fs *fileStream) ReadFrom(r io.Reader) (n int64, err error) {
	// Ensure that the directories in the path already exist
	dir := filepath.Dir(fs.path)
	err = os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return 0, fmt.Errorf("failed to create directory %s, %w", dir, err)
	}
	// Open the file with Append + Create + WriteOnly modes.
	// This ensures the file is created if it does not exist.
	// If the file does exist, data is appended instead of overwritten/truncated
	file, err := os.OpenFile(fs.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, fmt.Errorf("failed to open file %s: %w", fs.path, err)
	}
	defer func() {
		closeErr := file.Close()
		if err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	writer := bufio.NewWriterSize(file, fs.size)
	n, err = writer.ReadFrom(r)
	if err != nil {
		return
	}
	err = writer.Flush()
	return
}

func (fs *fileStream) Delete() error {
	return os.RemoveAll(fs.path)
}

func (fs *fileStream) Flush() error {
	return nil
}
