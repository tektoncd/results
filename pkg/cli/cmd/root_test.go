package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tektoncd/results/pkg/cli/dev/format"

	"github.com/google/go-cmp/cmp"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
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
		if err := format.PrintProto(b, in, kind); err != nil {
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

func TestPrintProto_Tab(t *testing.T) {
	ts := time.Now().Local().Truncate(time.Second)

	for _, tc := range []struct {
		m    proto.Message
		want [][]string
	}{
		{
			m: &pb.ListResultsResponse{
				Results: []*pb.Result{{
					Name:       "a",
					CreateTime: timestamppb.New(ts),
					UpdateTime: timestamppb.New(ts),
				}},
			},
			want: [][]string{
				{"Name", "Start", "Update"},
				{"a", ts.String(), ts.String()},
			},
		},
		{
			m: &pb.ListRecordsResponse{
				Records: []*pb.Record{{
					Name:       "a",
					CreateTime: timestamppb.New(ts),
					UpdateTime: timestamppb.New(ts),
					Data: &pb.Any{
						Type: "tacocat",
					},
				}},
			},
			want: [][]string{
				{"Name", "Type", "Start", "Update"},
				{"a", "tacocat", ts.String(), ts.String()},
			},
		},
	} {
		t.Run(fmt.Sprintf("%T", tc.m), func(t *testing.T) {
			// Dump output to buffer
			b := new(bytes.Buffer)
			if err := format.PrintProto(b, tc.m, "tab"); err != nil {
				t.Fatal(err)
			}

			// Normalize output by tokenizing each line to remove soft-tabs.
			var got [][]string
			scanner := bufio.NewScanner(b)
			for scanner.Scan() {
				line := []string{}
				// Tabwriter is configured for at least 2 spaces between each field.
				for _, s := range strings.Split(scanner.Text(), "  ") {
					// There might be multiple "tabs" for each field that
					// causes split to contain ""s - only look at non-empty
					// tokens.
					if s != "" {
						line = append(line, strings.TrimSpace(s))
					}
				}
				fmt.Println(line)
				got = append(got, line)
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Error(diff)
			}
		})
	}
}
