package log

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFileStream_WriteTo(t *testing.T) {
	f, err := os.CreateTemp("", "")
	if err != nil {
		t.Error(err)
	}
	defer f.Close()
	defer os.Remove(f.Name())

	want := "test data"
	if _, err = f.Write([]byte(want)); err != nil {
		t.Error(err)
	}

	stream := fileStream{
		path: f.Name(),
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

func TestFileStream_ReadFrom(t *testing.T) {
	want := []byte("test data")
	buffer := &bytes.Buffer{}
	buffer.Write(want)

	stream := fileStream{
		path: filepath.Join(os.TempDir(), "temp-file"),
	}
	defer os.Remove(stream.path)

	_, err := stream.ReadFrom(buffer)
	if err != nil {
		t.Error(err)
	}

	got, err := os.ReadFile(stream.path)
	if err != nil {
		t.Error(err)
	}
	if string(got) != string(want) {
		t.Errorf("want: %s, got: %s", want, got)
	}
}

func TestFileStream_Delete(t *testing.T) {
	f, err := os.CreateTemp("", "")
	if err != nil {
		t.Error(err)
	}
	defer f.Close()
	defer os.Remove(f.Name())

	stream := fileStream{
		path: f.Name(),
	}
	if err := stream.Delete(); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(f.Name()); !errors.Is(err, os.ErrNotExist) {
		t.Error(err)
	}
}
