package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"os"
	"time"

	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"github.com/tektoncd/results/tools/tkn-results/internal/config"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
	v1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
)

type ClientFactory struct {
	k8s kubernetes.Interface
	cfg *config.Config
}

func NewDefaultFactory() (*ClientFactory, error) {
	cfg := config.GetConfig()

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, nil)
	clientconfig, err := kubeconfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	if cfg.ServiceAccount != nil && cfg.ServiceAccount.Name != "" &&
		cfg.ServiceAccount.Namespace == "" {
		ns, _, err := kubeconfig.Namespace()
		if err != nil {
			return nil, err
		}
		cfg.ServiceAccount.Namespace = ns
	}
	client, err := kubernetes.NewForConfig(clientconfig)
	if err != nil {
		return nil, err
	}

	return &ClientFactory{
		k8s: client,
		cfg: cfg,
	}, nil
}

// ResultsClient creates a new Results gRPC client for the given factory settings.
// TODO: Refactor this with watcher client code?
func (f *ClientFactory) ResultsClient(ctx context.Context, overrideApiAddr string) (pb.ResultsClient, error) {
	token, err := f.token(ctx)
	if err != nil {
		return nil, err
	}

	var creds credentials.TransportCredentials
	if f.cfg.Insecure {
		creds = credentials.NewTLS(&tls.Config{
			InsecureSkipVerify: true,
		})
	} else {
		certs, err := f.certs()
		if err != nil {
			return nil, err
		}
		creds = credentials.NewClientTLSFromCert(certs, f.cfg.SSL.ServerNameOverride)
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	addr := f.cfg.Address
	if overrideApiAddr != "" {
		addr = overrideApiAddr
	}
	conn, err := grpc.DialContext(ctx, addr, grpc.WithBlock(),
		grpc.WithTransportCredentials(creds),
		grpc.WithDefaultCallOptions(grpc.PerRPCCredentials(oauth.TokenSource{
			TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}),
		})),
	)
	if err != nil {
		fmt.Printf("Dial: %v\n", err)
		return nil, err
	}
	return pb.NewResultsClient(conn), nil
}

// DefaultResultsClient creates a new results client.
// Will dial overrideApiAddr if overrideApiAddr is not empty
func DefaultResultsClient(ctx context.Context, overrideApiAddr string) (pb.ResultsClient, error) {
	f, err := NewDefaultFactory()

	if err != nil {
		return nil, err
	}

	client, err := f.ResultsClient(ctx, overrideApiAddr)

	if err != nil {
		return nil, err
	}

	return client, nil
}

// LogClient creates a new Results gRPC client for the given factory settings.
// TODO: Refactor this with watcher client code?
func (f *ClientFactory) LogClient(ctx context.Context, overrideApiAddr string) (pb.LogsClient, error) {
	token, err := f.token(ctx)
	if err != nil {
		return nil, err
	}

	var creds credentials.TransportCredentials
	if f.cfg.Insecure {
		creds = credentials.NewTLS(&tls.Config{
			InsecureSkipVerify: true,
		})
	} else {
		certs, err := f.certs()
		if err != nil {
			return nil, err
		}
		creds = credentials.NewClientTLSFromCert(certs, f.cfg.SSL.ServerNameOverride)
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	addr := f.cfg.Address
	if overrideApiAddr != "" {
		addr = overrideApiAddr
	}
	conn, err := grpc.DialContext(ctx, addr, grpc.WithBlock(),
		grpc.WithTransportCredentials(creds),
		grpc.WithDefaultCallOptions(grpc.PerRPCCredentials(oauth.TokenSource{
			TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}),
		})),
	)
	if err != nil {
		fmt.Printf("Dial: %v\n", err)
		return nil, err
	}
	return pb.NewLogsClient(conn), nil
}

func DefaultLogsClient(ctx context.Context, overrideApiAddr string) (pb.LogsClient, error) {
	f, err := NewDefaultFactory()

	if err != nil {
		return nil, err
	}

	client, err := f.LogClient(ctx, overrideApiAddr)

	if err != nil {
		return nil, err
	}

	return client, nil
}

func (f *ClientFactory) certs() (*x509.CertPool, error) {
	certs, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}
	if path := f.cfg.SSL.RootsFilePath; path != "" {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		b, err := io.ReadAll(f)
		if err != nil {
			return nil, fmt.Errorf("unable to read TLS cert file: %v", err)
		}
		if ok := certs.AppendCertsFromPEM(b); !ok {
			return nil, fmt.Errorf("unable to add cert to pool")
		}
	}
	return certs, nil
}

func (f *ClientFactory) token(ctx context.Context) (string, error) {
	if f.cfg == nil {
		return "", nil
	}

	if t := f.cfg.Token; t != "" {
		return t, nil
	}

	if sa := f.cfg.ServiceAccount; sa != nil {
		t, err := f.k8s.CoreV1().ServiceAccounts(sa.Namespace).CreateToken(ctx, sa.Name, &v1.TokenRequest{}, metav1.CreateOptions{})
		if err != nil {
			return "", fmt.Errorf("error getting service account token: %w", err)
		}
		return t.Status.Token, nil
	}

	return "", nil
}
