package log

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"
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
	expected := "Hello World!"
	_, err = tmp.WriteString("Hello World!")
	if err != nil {
		t.Fatalf("failed to write test message: %v", err)
	}
	trl := &TaskRunLog{
		Type: FileLogType,
		File: &FileLogTypeSpec{
			Path: tmp.Name(),
		},
	}
	streamer, err := NewFileLogStreamer(trl, 1024)
	if err != nil {
		t.Fatalf("failed to create file log streamer: %v", err)
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
