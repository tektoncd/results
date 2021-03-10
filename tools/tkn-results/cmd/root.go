package cmd

import (
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"time"

	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
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
	addr      *string = flag.StringP("addr", "a", "localhost:50051", "Result API server address")
	authToken *string = flag.StringP("authtoken", "t", "", "authorization bearer token to use for authenticated requests")

	RootCmd = &cobra.Command{
		Use:   "tkn-results",
		Short: "tkn CLI plugin for Tekton Results API",
		Long: `Environment Variables:
		TKN_RESULTS_SSL_ROOTS_FILE_PATH: Path to local SSL cert to use.
		TKN_RESULTS_SSL_SERVER_NAME_OVERRIDE: SSL server name override (useful if using with a proxy such as kubectl port-forward)`,
	}
)

// Execute executes the root command.
func Execute() error {
	return RootCmd.Execute()
}

// TODO: Refactor this with watcher client code?
func client(ctx context.Context) (pb.ResultsClient, error) {
	certs, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}
	if path := os.Getenv("TKN_RESULTS_SSL_ROOTS_FILE_PATH"); path != "" {
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
	conn, err := grpc.DialContext(ctx, *addr, grpc.WithBlock(),
		grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(certs, os.Getenv("TKN_RESULTS_SSL_SERVER_NAME_OVERRIDE"))),
		grpc.WithDefaultCallOptions(grpc.PerRPCCredentials(oauth.TokenSource{
			TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: *authToken}),
		})),
	)
	if err != nil {
		fmt.Printf("Dial: %v\n", err)
		return nil, err
	}
	return pb.NewResultsClient(conn), nil
}

func printproto(w io.Writer, pb proto.Message, format string) error {
	switch format {
	case "textproto":
		opts := prototext.MarshalOptions{
			Multiline: true,
		}
		b, err := opts.Marshal(pb)
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
		b, err := opts.Marshal(pb)
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
