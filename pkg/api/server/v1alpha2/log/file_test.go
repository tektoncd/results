package log

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"

	"github.com/tektoncd/results/pkg/apis/v1alpha2"
)

func TestFileLogStreamer(t *testing.T) {
	dir, err := ioutil.TempDir("", "test-filelogstreamer-*")
	if err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}
	tmp, err := ioutil.TempFile(dir, "temp-file-*.log")
	if err != nil {
		t.Fatalf("failed to create test log file: %v", err)
	}
	t.Logf("test log file: %s", tmp.Name())
	t.Cleanup(func() {
		err := tmp.Close()
		if err != nil {
			t.Fatalf("failed to close test log file: %v", err)
		}
		err = os.RemoveAll(dir)
		if err != nil {
			t.Fatalf("failed to remove directory %s: %v", dir, err)
		}
	})

	trl := &v1alpha2.TaskRunLog{
		Type: v1alpha2.FileLogType,
		File: &v1alpha2.FileLogTypeSpec{
			Path: tmp.Name(),
		},
	}
	streamer, err := NewFileLogStreamer(trl, 1024)
	if err != nil {
		t.Fatalf("failed to create file log streamer: %v", err)
	}

	expected := "Hello World!"
	buffer := &bytes.Buffer{}
	buffer.WriteString(expected)

	n, err := streamer.ReadFrom(buffer)
	if err != nil {
		t.Fatalf("ReadFrom: failed to read from buffer and write to storage: %v", err)
	}
	if n != int64(len(expected)) {
		t.Errorf("expected %d bytes to be read, got %d", len(expected), n)
	}

	outBuf := &bytes.Buffer{}
	_, err = streamer.WriteTo(outBuf)
	if err != nil {
		t.Fatalf("WriteTo: failed with error: %v", err)
	}
	actual := outBuf.String()
	if expected != actual {
		t.Errorf("WriteTo: expected %q, got %q", expected, actual)
	}
}
