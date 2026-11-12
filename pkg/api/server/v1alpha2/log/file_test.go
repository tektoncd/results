package log

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tektoncd/results/pkg/api/server/config"
	"github.com/tektoncd/results/pkg/apis/v1alpha3"
)

func newFileStream(t *testing.T, path string) fileStream {
	t.Helper()
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatalf("unable to create root dir for fileStream: %v", err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Errorf("unable to close root dir for fileStream: %v", err)
		}
	})
	return fileStream{
		root: root,
		path: path,
	}
}

func TestFileStream_WriteTo(t *testing.T) {
	stream := newFileStream(t, "")
	f, err := os.CreateTemp(stream.root.Name(), "")
	if err != nil {
		t.Error(err)
	}
	defer f.Close()
	stream.path = filepath.Base(f.Name())

	want := "test data"
	if _, err = f.Write([]byte(want)); err != nil {
		t.Error(err)
	}

	buffer := &bytes.Buffer{}
	_, err = stream.WriteTo(buffer)
	if err != nil {
		t.Error(err)
	}

	if got := buffer.String(); got != want {
		t.Errorf("want: %s, got: %s", want, got)
	}
}

// TestFileStream_WriteTo_BadPath tests potentially malicious paths which reach outside of
// the configured base directory, and ensures they result in an error
func TestFileStream_WriteTo_BadPath(t *testing.T) {
	// Setup the file system so paths can be referenced in the test case list
	actualTmpDir := t.TempDir()
	illegalTmpDir := t.TempDir()

	// File outside of the base directory
	badFile, err := os.CreateTemp(illegalTmpDir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer badFile.Close()
	badFile.WriteString("boom")
	relativePathToBadFile, err := filepath.Rel(actualTmpDir, badFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Symlink inside the base directory, but points to file outside of the base directory
	os.Symlink(badFile.Name(), filepath.Join(actualTmpDir, "symlink"))

	// "non-log" indicating it's not even in a temp directory;
	// the the file is not relative to the base directory at all
	badNonLog, err := os.CreateTemp(".", "temp-bad-file")
	if err != nil {
		t.Fatal(err)
	}
	defer badNonLog.Close()
	defer os.RemoveAll(badNonLog.Name())
	badNonLog.WriteString("boom")

	isPathEscapesErr := func(err error) bool {
		return err != nil && strings.Contains(err.Error(), "path escapes from parent")
	}

	testCases := []struct {
		name          string
		path          string
		deleteAllowed bool
	}{
		{name: "path to bad file", path: relativePathToBadFile},
		// you can delete a symlink
		{name: "path to symlink to bad file", path: "symlink", deleteAllowed: true},
		{name: "path to non-log", path: filepath.Join("../../../../../../../../", badNonLog.Name())},
	}

	newRealFileStream := func(t *testing.T, path string) Stream {
		fs, err := NewFileStream(t.Context(), &v1alpha3.Log{Status: v1alpha3.LogStatus{Path: path}}, &config.Config{LOGS_PATH: actualTmpDir})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = fs.Close() })
		return fs
	}

	t.Run("WriteTo", func(t *testing.T) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				fs := newRealFileStream(t, tc.path)
				buff := &bytes.Buffer{}
				if _, err := fs.WriteTo(buff); !isPathEscapesErr(err) {
					t.Fatalf("Should have errored with 'path escapes', got %v", err)
				}
			})
		}
	})

	t.Run("ReadFrom", func(t *testing.T) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				fs := newRealFileStream(t, tc.path)
				buff := &bytes.Buffer{}
				buff.WriteString("data")
				if _, err := fs.ReadFrom(buff); !isPathEscapesErr(err) {
					t.Fatalf("Should have errored with 'path escapes', got %v", err)
				}
			})
		}
	})

	t.Run("Delete", func(t *testing.T) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				fs := newRealFileStream(t, tc.path)
				t.Cleanup(func() { _ = fs.Close() })
				if err := fs.Delete(); !tc.deleteAllowed && !isPathEscapesErr(err) {
					t.Fatalf("Should have errored with 'path escapes', got %v", err)
				}
			})
		}
	})

}

func TestFileStream_ReadFrom(t *testing.T) {
	want := []byte("test data")
	buffer := &bytes.Buffer{}
	buffer.Write(want)

	stream := newFileStream(t, "temp-file")

	_, err := stream.ReadFrom(buffer)
	if err != nil {
		t.Error(err)
	}

	got, err := os.ReadFile(filepath.Join(stream.root.Name(), stream.path))
	if err != nil {
		t.Error(err)
	}
	if string(got) != string(want) {
		t.Errorf("want: %s, got: %s", want, got)
	}
}

func TestFileStream_Delete(t *testing.T) {
	stream := newFileStream(t, "")
	f, err := os.CreateTemp(stream.root.Name(), "")
	if err != nil {
		t.Error(err)
	}
	defer f.Close()
	stream.path = filepath.Base(f.Name())

	if err := stream.Delete(); err != nil {
		t.Errorf("Error deleting file: %v", err)
	}
	if _, err := os.Stat(f.Name()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("File should not exist. Got: %v", err)
	}
}

// TestNewFileStream_CreatesMissingRoot ensures streams can be created when the
// configured logs root directory does not exist yet, matching the previous
// behavior where writing a log lazily created its directory.
func TestNewFileStream_CreatesMissingRoot(t *testing.T) {
	logsPath := filepath.Join(t.TempDir(), "does", "not", "exist")
	stream, err := NewFileStream(t.Context(), &v1alpha3.Log{
		Status: v1alpha3.LogStatus{Path: "foo/bar.log"},
	}, &config.Config{LOGS_PATH: logsPath})
	if err != nil {
		t.Fatalf("expected stream creation to succeed with missing logs root: %v", err)
	}
	t.Cleanup(func() { _ = stream.Close() })

	buffer := &bytes.Buffer{}
	buffer.WriteString("test data")
	if _, err := stream.ReadFrom(buffer); err != nil {
		t.Fatalf("expected log write to succeed: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(logsPath, "foo", "bar.log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "test data" {
		t.Errorf("want: %s, got: %s", "test data", got)
	}
}
