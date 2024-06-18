package client

import (
	"context"
	"fmt"
	"net/url"
	"time"

	resultsv1alpha2 "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"k8s.io/client-go/transport"
)

// GRPCClient represents GRPC API client to connect to Tekton results api server.
type GRPCClient interface {
	resultsv1alpha2.LogsClient
	resultsv1alpha2.ResultsClient
}

type grpcClient struct {
	resultsv1alpha2.LogsClient
	resultsv1alpha2.ResultsClient
}

// NewGRPCClient creates a new gRPC client.
func NewGRPCClient(serverAddress string, opts ...grpc.DialOption) (GRPCClient, error) {
	u, err := url.Parse(serverAddress)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// target := net.JoinHostPort(u.Hostname(), u.Port())
	clientConn, err := grpc.DialContext(ctx, u.Host, opts...) //nolint:staticcheck
	if err != nil {
		return nil, err
	}

	return &grpcClient{
		resultsv1alpha2.NewLogsClient(clientConn),
		resultsv1alpha2.NewResultsClient(clientConn),
	}, nil
}

// CustomCredentials supplies PerRPCCredentials from a Token Source and Impersonation config.
type CustomCredentials struct {
	oauth2.TokenSource
	*transport.ImpersonationConfig
}

// GetRequestMetadata gets the request metadata as a map from a Custom.
func (cc *CustomCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) { //nolint:revive
	ri, _ := credentials.RequestInfoFromContext(ctx)
	if err := credentials.CheckSecurityLevel(ri.AuthInfo, credentials.PrivacyAndIntegrity); err != nil {
		return nil, fmt.Errorf("unable to transfer TokenSource PerRPCCredentials: %v", err)
	}

	token, err := cc.Token()
	if err != nil {
		return nil, err
	}

	m := map[string]string{
		"authorization": token.Type() + " " + token.AccessToken,
	}
	if cc.UserName != "" {
		m[transport.ImpersonateUserHeader] = cc.UserName
	}
	if cc.UID != "" {
		m[transport.ImpersonateUIDHeader] = cc.UID
	}
	for _, group := range cc.Groups {
		m[transport.ImpersonateUIDHeader] = group
	}
	for ek, ev := range cc.Extra {
		for _, v := range ev {
			m[transport.ImpersonateUserExtraHeaderPrefix+unescapeExtraKey(ek)] = v
		}
	}

	return m, nil
}

// RequireTransportSecurity indicates whether the credentials requires transport security.
func (cc *CustomCredentials) RequireTransportSecurity() bool {
	return true
}

func unescapeExtraKey(encodedKey string) string {
	key, err := url.PathUnescape(encodedKey) // Decode %-encoded bytes.
	if err != nil {
		return encodedKey // Always record extra strings, even if malformed/unencoded.
	}
	return key
}
