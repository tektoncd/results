package format

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

// PrintProto prints the given proto message to the given writer in the given format.
// Valid formats are: tab, textproto, json
func PrintProto(w io.Writer, m proto.Message, format string) error {
	switch format {
	case "tab":
		tw := tabwriter.NewWriter(w, 40, 2, 2, ' ', 0)
		switch t := m.(type) {
		case *pb.ListResultsResponse:
			fmt.Fprintln(tw, strings.Join([]string{"Name", "Start", "Update"}, "\t"))
			for _, r := range t.GetResults() {
				fmt.Fprintln(tw, strings.Join([]string{
					r.GetName(),
					r.GetCreateTime().AsTime().Truncate(time.Second).Local().String(),
					r.GetUpdateTime().AsTime().Truncate(time.Second).Local().String(),
				}, "\t"))
			}
		case *pb.ListRecordsResponse:
			fmt.Fprintln(tw, strings.Join([]string{"Name", "Type", "Start", "Update"}, "\t"))
			for _, r := range t.GetRecords() {
				fmt.Fprintln(tw, strings.Join([]string{
					r.GetName(),
					r.GetData().GetType(),
					r.GetCreateTime().AsTime().Truncate(time.Second).Local().String(),
					r.GetUpdateTime().AsTime().Truncate(time.Second).Local().String(),
				}, "\t"))
			}
		}
		tw.Flush()
	case "textproto":
		opts := prototext.MarshalOptions{
			Multiline: true,
		}
		b, err := opts.Marshal(m)
		if err != nil {
			return err
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
	case "json":
		opts := protojson.MarshalOptions{
			Multiline: true,
		}
		b, err := opts.Marshal(m)
		if err != nil {
			return err
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown output format %q", format)
	}
	return nil
}
