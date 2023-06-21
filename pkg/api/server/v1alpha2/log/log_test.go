package log

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"github.com/tektoncd/results/pkg/api/server/config"
	"github.com/tektoncd/results/pkg/api/server/db"
	"github.com/tektoncd/results/pkg/apis/v1alpha2"
	"github.com/tektoncd/results/pkg/internal/jsonutil"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFilePath(t *testing.T) {
	log := &v1alpha2.Log{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-log",
			Namespace: "test",
			UID:       "test-uid",
		},
	}
	want := "test/test-uid/test-log"
	got, err := FilePath(log)
	if err != nil {
		t.Error(err)
	}
	if got != want {
		t.Errorf("want %s, got %s", want, got)
	}
}

func TestFormatName(t *testing.T) {
	got := FormatName("a", "b")
	want := "a/logs/b"
	if want != got {
		t.Errorf("want %s, got %s", want, got)
	}
}

func TestParseName(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		// if want is nil, assume error
		want []string
	}{
		{
			name: "simple",
			in:   "a/results/b/logs/c",
			want: []string{"a", "b", "c"},
		},
		{
			name: "resource name reuse",
			in:   "results/results/logs/logs/logs",
			want: []string{"results", "logs", "logs"},
		},
		{
			name: "missing name",
			in:   "a/results/b/logs/",
		},
		{
			name: "missing name, no slash",
			in:   "a/results/b/logs/",
		},
		{
			name: "missing parent",
			in:   "/logs/b",
		},
		{
			name: "missing parent, no slash",
			in:   "logs/b",
		},
		{
			name: "wrong resource",
			in:   "a/tacocat/b/logs/c",
		},
		{
			name: "result resource",
			in:   "a/results/b",
		},
		{
			name: "invalid parent",
			in:   "a/b/results/c",
		},
		{
			name: "invalid name",
			in:   "a/results/b/logs/c/d",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent, result, name, err := ParseName(tc.in)
			if err != nil {
				if tc.want == nil {
					// error was expected, continue
					return
				}
				t.Fatal(err)
			}
			if tc.want == nil {
				t.Fatalf("expected error, got: [%s, %s]", parent, name)
			}
			if parent != tc.want[0] || result != tc.want[1] || name != tc.want[2] {
				t.Errorf("want: %v, got: [%s, %s, %s]", tc.want, parent, result, name)
			}
		})
	}
}

func TestToStorage(t *testing.T) {
	log := &v1alpha2.Log{
		ObjectMeta: v1.ObjectMeta{
			Name: "test-taskrun-log",
		},
		Spec: v1alpha2.LogSpec{
			Type: v1alpha2.FileLogType,
			Resource: v1alpha2.Resource{
				Name: "test-taskrun",
			},
		},
	}
	log.Default()

	want, err := json.Marshal(log)
	if err != nil {
		t.Error(err)
	}

	rec := &pb.Record{
		Name: "test-log",
		Data: &pb.Any{
			Type:  v1alpha2.LogRecordType,
			Value: want,
		},
	}

	got, err := ToStorage(rec, &config.Config{})
	if bytes.Compare(got, want) != 0 { //nolint:gosimple
		t.Error(err)
	}
}

type mockStream struct {
	streamType string
}

func (s *mockStream) WriteTo(io.Writer) (int64, error) {
	return 0, fmt.Errorf("not implemented")
}

func (s *mockStream) ReadFrom(io.Reader) (int64, error) {
	return 0, fmt.Errorf("not implemented")
}

func (s *mockStream) Type() string {
	return s.streamType
}

func (s *mockStream) Delete() error {
	return fmt.Errorf("not implemented")
}

func (s *mockStream) Flush() error {
	return fmt.Errorf("not implemented")
}

func TestToStream(t *testing.T) {
	cases := []struct {
		name      string
		in        *db.Record
		want      Stream
		expectErr bool
	}{
		{
			name: "Log Filesystem Type",
			in: &db.Record{
				Parent:     "app",
				ResultID:   "1",
				ResultName: "push-main",
				Name:       "taskrun-compile-log",
				ID:         "a",
				Type:       v1alpha2.LogRecordType,
				Data: jsonutil.AnyBytes(t, &v1alpha2.Log{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-log",
						Namespace: "test",
						UID:       "test-uid",
					},
					Spec: v1alpha2.LogSpec{
						Type: v1alpha2.FileLogType,
						Resource: v1alpha2.Resource{
							Namespace: "app",
							Name:      "taskrun-compile",
						},
					},
				}),
			},
			want: &mockStream{
				streamType: string(v1alpha2.FileLogType),
			},
		},
		{
			name: "TaskRun Record",
			in: &db.Record{
				Parent:     "app",
				ResultID:   "1",
				ResultName: "push-main",
				Name:       "taskrun-compile",
				ID:         "a",
				Type:       "pipeline.tekton.dev/TaskRun",
			},
			expectErr: true,
		},
		{
			name: "PipelineRun Record",
			in: &db.Record{
				Parent:     "app",
				ResultID:   "1",
				ResultName: "push-main",
				Name:       "taskrun-compile",
				ID:         "a",
				Type:       "pipeline.tekton.dev/PipelineRun",
			},
			expectErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			streamer, _, err := ToStream(context.TODO(), tc.in, &config.Config{})
			if err != nil {
				if !tc.expectErr {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if streamer.Type() != tc.want.Type() {
				t.Errorf("expected log streamer %s, got %s", tc.want.Type(), streamer.Type())
			}
		})
	}
}
