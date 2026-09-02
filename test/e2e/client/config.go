package client

import (
	"fmt"
	"os"
	"path"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"k8s.io/client-go/transport"
)

// Default e2e environment values. These match the paths used by
// test/e2e/01-install.sh and the kind cluster setup scripts.
const (
	DefaultServerName    = "tekton-results-api-service.tekton-pipelines.svc.cluster.local"
	DefaultServerAddress = "https://localhost:8080"
	DefaultCertFileName  = "tekton-results-cert.pem"
	DefaultCertPath      = "/tmp/tekton-results/ssl"
	DefaultTokenPath     = "/tmp/tekton-results/tokens" //nolint:gosec // Not a credential; directory path for SA token files.

	AdminTokenFile = "all-namespaces-admin-access"
	ReadTokenFile  = "all-namespaces-read-access"
)

// EnvConfig holds the resolved paths and addresses for the e2e test environment.
type EnvConfig struct {
	CertFile      string
	TokenPath     string
	ServerName    string
	ServerAddress string
}

// NewEnvConfig reads the standard e2e environment variables and falls back to
// defaults. Both the main e2e suite and the db sub-suite share this to avoid
// duplicating the env-var-to-default resolution logic.
func NewEnvConfig() EnvConfig {
	certPath := EnvOrDefault("SSL_CERT_PATH", DefaultCertPath)
	certFileName := EnvOrDefault("CERT_FILE_NAME", DefaultCertFileName)
	return EnvConfig{
		CertFile:      path.Join(certPath, certFileName),
		TokenPath:     EnvOrDefault("SA_TOKEN_PATH", DefaultTokenPath),
		ServerName:    EnvOrDefault("API_SERVER_NAME", DefaultServerName),
		ServerAddress: EnvOrDefault("API_SERVER_ADDR", DefaultServerAddress),
	}
}

// TokenFile returns the full path for a given token file name under the
// resolved token directory.
func (c EnvConfig) TokenFile(name string) string {
	return path.Join(c.TokenPath, name)
}

// NewGRPCClientFromConfig creates a GRPCClient using the shared EnvConfig and
// the specified token file name.
func NewGRPCClientFromConfig(cfg EnvConfig, tokenFileName string, impersonationConfig *transport.ImpersonationConfig) (GRPCClient, error) {
	if impersonationConfig == nil {
		impersonationConfig = &transport.ImpersonationConfig{}
	}

	transportCreds, err := credentials.NewClientTLSFromFile(cfg.CertFile, cfg.ServerName)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS credentials from %s: %w", cfg.CertFile, err)
	}

	opts := []grpc.DialOption{
		grpc.WithBlock(), //nolint:staticcheck
		grpc.WithTransportCredentials(transportCreds),
		grpc.WithDefaultCallOptions(grpc.PerRPCCredentials(&CustomCredentials{
			TokenSource:         transport.NewCachedFileTokenSource(cfg.TokenFile(tokenFileName)),
			ImpersonationConfig: impersonationConfig,
		})),
	}

	return NewGRPCClient(cfg.ServerAddress, opts...)
}

// NewRESTClientFromConfig creates a RESTClient using the shared EnvConfig and
// the specified token file name.
func NewRESTClientFromConfig(cfg EnvConfig, tokenFileName string, impersonationConfig *transport.ImpersonationConfig) (RESTClient, error) {
	if impersonationConfig == nil {
		impersonationConfig = &transport.ImpersonationConfig{}
	}

	if _, err := credentials.NewClientTLSFromFile(cfg.CertFile, cfg.ServerName); err != nil {
		return nil, fmt.Errorf("failed to verify TLS cert from %s: %w", cfg.CertFile, err)
	}

	restConfig := &transport.Config{
		TLS: transport.TLSConfig{
			CAFile:     cfg.CertFile,
			ServerName: cfg.ServerName,
		},
		BearerTokenFile: cfg.TokenFile(tokenFileName),
		Impersonate:     *impersonationConfig,
	}

	return NewRESTClient(cfg.ServerAddress, WithConfig(restConfig))
}

// EnvOrDefault returns the value of the environment variable named by key,
// or fallback if the variable is empty or unset.
func EnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
