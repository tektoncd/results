package cmd

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	clientutil "github.com/tektoncd/results/tools/tkn-results/internal/client"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	// TODO: Dynamically discover other protos to allow custom record printing.
	_ "github.com/tektoncd/results/proto/pipeline/v1beta1/pipeline_go_proto"
)

var (
	//go:embed help.txt
	help string

	RootCmd = &cobra.Command{
		Use:   "tkn-results",
		Short: "tkn CLI plugin for Tekton Results API",
		Long:  help,
	}
)

// Execute executes the root command.
func Execute() error {
	return RootCmd.Execute()
}

func client(ctx context.Context) (pb.ResultsClient, error) {
	f, err := clientutil.NewDefaultFactory()
	if err != nil {
		return nil, err
	}
	return f.Client(ctx)
}

func printproto(w io.Writer, m proto.Message, format string) error {
	switch format {
	case "tab":
		tw := tabwriter.NewWriter(w, 40, 2, 2, ' ', 0)
		switch t := m.(type) {
		case *pb.ListResultsResponse:
			fmt.Fprintln(tw, strings.Join([]string{"Name", "Start", "Update"}, "\t"))
			for _, r := range t.GetResults() {
				fmt.Fprintln(tw, strings.Join([]string{
					r.GetName(),
					r.GetCreatedTime().AsTime().Truncate(time.Second).Local().String(),
					r.GetUpdatedTime().AsTime().Truncate(time.Second).Local().String(),
				}, "\t"))
			}
		case *pb.ListRecordsResponse:
			fmt.Fprintln(tw, strings.Join([]string{"Name", "Type", "Start", "Update"}, "\t"))
			for _, r := range t.GetRecords() {
				fmt.Fprintln(tw, strings.Join([]string{
					r.GetName(),
					r.GetData().GetType(),
					r.GetCreatedTime().AsTime().Truncate(time.Second).Local().String(),
					r.GetUpdatedTime().AsTime().Truncate(time.Second).Local().String(),
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
