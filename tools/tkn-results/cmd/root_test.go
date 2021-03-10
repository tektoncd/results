package cmd

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestPrintProto(t *testing.T) {
	for kind, unmarshal := range map[string]func([]byte, protoreflect.ProtoMessage) error{
		"textproto": prototext.Unmarshal,
		"json":      protojson.Unmarshal,
	} {
		testPrintProto(t, kind, unmarshal, &pb.Result{Name: "a"}, &pb.Result{})
		testPrintProto(t, kind, unmarshal, &pb.Record{Name: "a"}, &pb.Record{})
	}
}

func testPrintProto(t *testing.T, kind string, unmarshal func([]byte, protoreflect.ProtoMessage) error, in, out proto.Message) {
	t.Run(fmt.Sprintf("%T_%s", in, kind), func(t *testing.T) {
		// Print out message to buffer.
		b := new(bytes.Buffer)
		if err := printproto(b, in, kind); err != nil {
			t.Fatalf("printproto: %v", err)
		}

		// Read buffer back into same message type.
		if err := unmarshal(b.Bytes(), out); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}

		// Make sure we didn't lose any data.
		if diff := cmp.Diff(in, out, protocmp.Transform()); diff != "" {
			t.Fatal(diff)
		}
	})
}
