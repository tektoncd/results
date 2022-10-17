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
	if err != nil {
		t.Fatalf("failed to create test log file: %v", err)
	}
	t.Logf("test log directory: %s", dir)
	t.Cleanup(func() {
		err = os.RemoveAll(dir)
		if err != nil {
			t.Fatalf("failed to remove directory %s: %v", dir, err)
		}
	})

	trl := &v1alpha2.TaskRunLog{
		Spec: v1alpha2.TaskRunLogSpec{
			Type: v1alpha2.FileLogType,
			Ref: v1alpha2.TaskRunRef{
				Namespace: "filelogstream",
				Name:      "build-file",
			},
		},
	}
	streamer := NewFileLogStreamer(trl, 1024, dir)
	if err != nil {
		t.Fatalf("failed to create file log streamer: %v", err)
	}
	// Verify that the taskRunLog has the right path
	if trl.Status.File == nil {
		t.Error("expected TaskRunLog record to have a file path in its status.")
	} else {
		if len(trl.Status.File.Path) == 0 {
			t.Error("expected TaskRunLog record to have a file path in its status.")
		}
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
