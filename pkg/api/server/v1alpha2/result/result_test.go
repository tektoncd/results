// Copyright 2020 The Tekton Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package result

import (
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"knative.dev/pkg/ptr"

	cw "github.com/jonboulle/clockwork"
	"github.com/tektoncd/results/pkg/api/server/cel"
	"github.com/tektoncd/results/pkg/api/server/db"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var clock cw.Clock = cw.NewFakeClock()

func TestParseName(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		// if want is nil, assume error
		want []string
	}{
		{
			name: "simple",
			in:   "a/results/b",
			want: []string{"a", "b"},
		},
		{
			name: "resource name reuse",
			in:   "results/results/results",
			want: []string{"results", "results"},
		},
		{
			name: "missing name",
			in:   "a/results/",
		},
		{
			name: "missing name, no slash",
			in:   "a/results",
		},
		{
			name: "missing parent",
			in:   "/results/b",
		},
		{
			name: "missing parent, no slash",
			in:   "results/b",
		},
		{
			name: "wrong resource",
			in:   "a/record/b",
		},
		{
			name: "invalid parent",
			in:   "a/b/results/c",
		},
		{
			name: "invalid name",
			in:   "a/results/b/c",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent, name, err := ParseName(tc.in)
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

			if parent != tc.want[0] || name != tc.want[1] {
				t.Errorf("want: %v, got: [%s, %s]", tc.want, parent, name)
			}
		})
	}
}

func TestToStorage(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   *pb.Result
		want *db.Result
	}{
		{
			name: "all",
			in: &pb.Result{
				Name:        "foo/results/bar",
				Id:          "a",
				CreatedTime: timestamppb.New(clock.Now()),
				UpdatedTime: timestamppb.New(clock.Now()),
				Annotations: map[string]string{"a": "b"},
				Etag:        "tacocat",
				Summary: &pb.RecordSummary{
					Record:      "foo/results/bar/records/baz",
					Type:        "bar",
					StartTime:   timestamppb.New(clock.Now()),
					EndTime:     timestamppb.New(clock.Now()),
					Status:      pb.RecordSummary_SUCCESS,
					Annotations: map[string]string{"c": "d"},
				},
			},
			want: &db.Result{
				Parent:      "foo",
				Name:        "bar",
				ID:          "a",
				Annotations: map[string]string{"a": "b"},
				CreatedTime: clock.Now(),
				UpdatedTime: clock.Now(),
				Etag:        "tacocat",
				Summary: db.RecordSummary{
					Record:      "foo/results/bar/records/baz",
					Type:        "bar",
					StartTime:   ptr.Time(clock.Now()),
					EndTime:     ptr.Time(clock.Now()),
					Status:      1,
					Annotations: map[string]string{"c": "d"},
				},
			},
		},
		{
			name: "deprecated fields",
			in: &pb.Result{
				Name:        "foo/results/bar",
				Uid:         "a",
				Id:          "b",
				CreatedTime: timestamppb.New(clock.Now().Add(time.Minute)),
				CreateTime:  timestamppb.New(clock.Now()),
				UpdatedTime: timestamppb.New(clock.Now().Add(time.Minute)),
				UpdateTime:  timestamppb.New(clock.Now()),
				Annotations: map[string]string{"a": "b"},
				Etag:        "tacocat",
			},
			want: &db.Result{
				Parent:      "foo",
				Name:        "bar",
				ID:          "a",
				Annotations: map[string]string{"a": "b"},
				CreatedTime: clock.Now(),
				UpdatedTime: clock.Now(),
				Etag:        "tacocat",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ToStorage(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("-want,+got: %s", diff)
			}
		})
	}

	// errors
	for _, tc := range []struct {
		name string
		in   *pb.Result
		want codes.Code
	}{
		{
			name: "invalid summary record name",
			in: &pb.Result{
				Name: "foo/results/bar",
				Id:   "a",
				Summary: &pb.RecordSummary{
					Record: "foo",
				},
			},
			want: codes.InvalidArgument,
		},
		{
			name: "invalid summary type",
			in: &pb.Result{
				Name: "foo/results/bar",
				Id:   "a",
				Summary: &pb.RecordSummary{
					Record: "foo/results/bar/records/baz",
					Type:   strings.Repeat("a", 1024),
				},
			},
			want: codes.InvalidArgument,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ToStorage(tc.in)
			if status.Code(err) != tc.want {
				t.Fatalf("expected %v, got (%v, %v)", tc.want, got, err)
			}
		})
	}
}

func TestToAPI(t *testing.T) {
	ann := map[string]string{"a": "b"}
	got := ToAPI(&db.Result{
		Parent:      "foo",
		Name:        "bar",
		ID:          "a",
		CreatedTime: clock.Now(),
		UpdatedTime: clock.Now(),
		Annotations: ann,
		Etag:        "etag",
	})
	want := &pb.Result{
		Name:        "foo/results/bar",
		Id:          "a",
		Uid:         "a",
		CreatedTime: timestamppb.New(clock.Now()),
		CreateTime:  timestamppb.New(clock.Now()),
		UpdatedTime: timestamppb.New(clock.Now()),
		UpdateTime:  timestamppb.New(clock.Now()),
		Annotations: ann,
		Etag:        "etag",
	}
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("-want,+got: %s", diff)
	}
}

func TestMatch(t *testing.T) {
	env, err := cel.NewEnv()
	if err != nil {
		t.Fatalf("NewEnv: %v", err)
	}

	r := &pb.Result{
		Name:        "foo",
		Id:          "bar",
		CreatedTime: timestamppb.Now(),
		Annotations: map[string]string{"a": "b"},
		Etag:        "tacocat",
	}
	for _, tc := range []struct {
		name   string
		result *pb.Result
		filter string
		match  bool
		status codes.Code
	}{
		{
			name:   "no filter",
			filter: "",
			result: r,
			match:  true,
		},
		{
			name:   "matching condition",
			filter: `result.id != ""`,
			result: r,
			match:  true,
		},
		{
			name:   "non-matching condition",
			filter: `result.id == ""`,
			result: r,
			match:  false,
		},
		{
			name:   "nil result",
			result: nil,
			filter: "result.id",
			match:  false,
		},
		{
			name:   "non-bool output",
			result: r,
			filter: "result",
			status: codes.InvalidArgument,
		},
		{
			name:   "wrong resource type",
			result: r,
			filter: "record",
			status: codes.InvalidArgument,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := cel.ParseFilter(env, tc.filter)
			if err != nil {
				t.Fatalf("ParseFilter: %v", err)
			}
			got, err := Match(tc.result, p)
			if status.Code(err) != tc.status {
				t.Fatalf("Match: %v", err)
			}
			if got != tc.match {
				t.Errorf("want: %t, got: %t", tc.match, got)
			}
		})
	}
}

func TestFormatName(t *testing.T) {
	got := FormatName("a", "b")
	want := "a/results/b"
	if want != got {
		t.Errorf("want %s, got %s", want, got)
	}
}
