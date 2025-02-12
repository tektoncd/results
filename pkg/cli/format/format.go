package format

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/hako/durafmt"
	"github.com/jonboulle/clockwork"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
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

// Age returns the age of the given timestamp in a human-readable format.
func Age(timestamp *timestamppb.Timestamp, c clockwork.Clock) string {
	if timestamp == nil {
		return "---"
	}
	t := timestamp.AsTime()
	if t.IsZero() {
		return "---"
	}
	duration := c.Since(t)
	return durafmt.ParseShort(duration).String() + " ago"
}

// Duration returns the duration between two timestamps in a human-readable format.
func Duration(timestamp1, timestamp2 *timestamppb.Timestamp) string {
	if timestamp1 == nil || timestamp2 == nil {
		return "---"
	}
	t1 := timestamp1.AsTime()
	t2 := timestamp2.AsTime()
	if t1.IsZero() || t2.IsZero() {
		return "---"
	}
	duration := t2.Sub(t1)
	return duration.String()
}

// Status returns the status of the given record summary in a human-readable format.
func Status(status pb.RecordSummary_Status) string {
	switch status {
	case pb.RecordSummary_SUCCESS:
		return "Succeeded"
	case pb.RecordSummary_FAILURE:
		return "Failed"
	case pb.RecordSummary_TIMEOUT:
		return "Timed Out"
	case pb.RecordSummary_CANCELLED:
		return "Cancelled"
	}
	return "Unknown"
}

// Namespace returns the namespace of the given result name.
func Namespace(resultName string) string {
	return strings.Split(resultName, "/")[0]
}
