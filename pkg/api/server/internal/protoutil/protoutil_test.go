package protoutil

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestClearOutputOnly(t *testing.T) {
	m := &pb.Result{
		Name:        "a",
		Id:          "b",
		CreatedTime: timestamppb.Now(),
		UpdatedTime: timestamppb.Now(),
		Annotations: map[string]string{"c": "d"},
		Etag:        "f",
	}
	want := &pb.Result{
		Name:        m.Name,
		Annotations: m.Annotations,
	}

	ClearOutputOnly(m)

	if diff := cmp.Diff(want, m, protocmp.Transform()); diff != "" {
		t.Errorf("-want, +got: %s", diff)
	}
}
