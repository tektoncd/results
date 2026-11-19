package fieldmask

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/tektoncd/results/internal/fieldmask/test"
	"github.com/tidwall/gjson"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

var p = []string{"a.b", "a.b.c", "d.e", "f"}

var m = metadata.New(map[string]string{
	"fields": strings.Join(p, ","),
})

var fm = FieldMask{
	"a": FieldMask{
		"b": {},
	},
	"d": FieldMask{
		"e": {},
	},
	"f": {},
}

var j = `
{ 
	"a": { 
		"b": { 
			"c": "test value" 
		} 
	}, 
	"d": { 
		"e": "test value" 
	}, 
	"g": {
		"h": "test value"
	}
}`

var pm = &test.Test{
	Id:   "test-id",
	Name: "test-name",
	Data: []*test.Any{
		{
			Type:  "type-1",
			Value: []byte(gjson.Parse(j).String()),
		},
		{
			Type:  "type-2",
			Value: []byte(gjson.Parse(j).String()),
		},
	},
}

func TestFieldMask_Build(t *testing.T) {
	f := &fieldmaskpb.FieldMask{Paths: p}
	f.Normalize()
	want := fm
	got := FieldMask{}
	got.Build(f.Paths)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Fieldmask mismatch (-want +got):\n%s", diff)
	}
}

func TestFieldMask_Paths(t *testing.T) {
	f := &fieldmaskpb.FieldMask{Paths: p}
	f.Normalize()
	want := f.Paths
	slices.Sort(want)
	got := fm.Paths(nil)
	slices.Sort(got)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Paths mismatch (-want +got):\n%s", diff)
	}
}

func TestFieldMask_Filter(t *testing.T) {
	f := FieldMask{}
	f.Build([]string{"name", "data.value.d"})
	got := proto.Clone(pm)
	f.Filter(got)
	d := gjson.Parse(`{"d":{"e":"test value"}}`).String()
	want := &test.Test{
		Name: "test-name",
		Data: []*test.Any{
			{Value: []byte(d)},
			{Value: []byte(d)},
		},
	}
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("Proto mismatch (-want +got):\n%s", diff)
	}
}

func TestFieldMask_FilterJSON(t *testing.T) {
	want := gjson.Parse(`{"a":{"b":{"c":"test value"}}}`).Value()
	f := FieldMask{}
	f.Build([]string{"a.b"})
	got := f.FilterJSON([]byte(j), []string{})

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("JSON mismatch (-want +got):\n%s", diff)
	}
}

func TestFromMetadata(t *testing.T) {
	want := fm
	got := FromMetadata(m)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Fieldmask mismatch (-want +got):\n%s", diff)
	}
}

func TestMetadataAnnotator(t *testing.T) {
	want := m
	got := MetadataAnnotator(context.Background(), &http.Request{
		Form: url.Values{
			"fields": m.Get("fields"),
		},
	})
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Metadata mismatch (-want +got):\n%s", diff)
	}
}

func TestForwardResponseFilter(t *testing.T) {
	var enabled atomic.Bool
	for _, method := range []string{"/tekton.results.v1alpha2.Results/GetRecord", "/tekton.results.v1alpha2.Logs/ListLogs"} {
		for _, on := range []bool{false, true} {
			enabled.Store(on)
			ctx, err := runtime.AnnotateIncomingContext(context.Background(), runtime.NewServeMux(), httptest.NewRequest("GET", "/", nil), method)
			if err != nil {
				t.Fatal(err)
			}
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("fields", "name", "fields", "data.value.d"))
			got := proto.Clone(pm)
			want := proto.Clone(pm)
			if on && strings.Contains(method, ".Results/") {
				FromMetadata(metadata.Pairs("fields", "name", "fields", "data.value.d")).Filter(want)
			}
			if err := ForwardResponseFilter(&enabled)(ctx, httptest.NewRecorder(), got); err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
				t.Fatalf("method=%s enabled=%v: %s", method, on, diff)
			}
			if err := ForwardResponseFilter(&enabled)(ctx, httptest.NewRecorder(), nil); err != nil {
				t.Fatal(err)
			}
		}
	}
}
