package cmd

import (
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"github.com/tektoncd/results/tools/tkn-results/internal/config"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	// TODO: Dynamically discover other protos to allow custom record printing.
	_ "github.com/tektoncd/results/proto/pipeline/v1beta1/pipeline_go_proto"
)

var (
	RootCmd = &cobra.Command{
		Use:   "tkn-results",
		Short: "tkn CLI plugin for Tekton Results API",
		Long:  config.EnvVarHelp(),
	}
)

// Execute executes the root command.
func Execute() error {
	return RootCmd.Execute()
}

// TODO: Refactor this with watcher client code?
func client(ctx context.Context) (pb.ResultsClient, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	certs, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}
	if path := cfg.SSL.RootsFilePath; path != "" {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		b, err := ioutil.ReadAll(f)
		if err != nil {
			return nil, fmt.Errorf("unable to read TLS cert file: %v", err)
		}
		if ok := certs.AppendCertsFromPEM(b); !ok {
			return nil, fmt.Errorf("unable to add cert to pool")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, cfg.Address, grpc.WithBlock(),
		grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(certs, cfg.SSL.ServerNameOverride)),
		grpc.WithDefaultCallOptions(grpc.PerRPCCredentials(oauth.TokenSource{
			TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: cfg.Token}),
		})),
	)
	if err != nil {
		fmt.Printf("Dial: %v\n", err)
		return nil, err
	}
	return pb.NewResultsClient(conn), nil
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
					r.GetData().GetTypeUrl(),
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
