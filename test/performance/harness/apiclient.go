/*
Copyright 2026 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"crypto/tls"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"k8s.io/client-go/transport"

	"github.com/tektoncd/results/test/e2e/client"
)

// APIClients bundles the transport clients the harness drives. The store driver
// uses gRPC; the query driver can use either transport.
type APIClients struct {
	GRPC client.GRPCClient
	REST client.RESTClient
}

// ClientConfig locates and authenticates against the deployed API server. The
// TLS + bearer-token construction mirrors test/e2e (resultsClient) so the
// harness talks to the real API surface the same way conformance tests do.
type ClientConfig struct {
	ServerAddress string // e.g. https://localhost:8080
	ServerName    string // TLS server name (cert CN/SAN)
	CertFile      string // CA cert file; empty means skip verification
	TokenFile     string // bearer token file (service-account token)
}

// NewAPIClients constructs the gRPC and REST clients from cfg. When CertFile is
// empty, TLS verification is skipped (convenient for a local kind cluster with a
// self-signed cert).
func NewAPIClients(cfg ClientConfig) (*APIClients, error) {
	var (
		tlsConfig transport.TLSConfig
		creds     credentials.TransportCredentials
	)
	if cfg.CertFile != "" {
		tc, err := credentials.NewClientTLSFromFile(cfg.CertFile, cfg.ServerName)
		if err != nil {
			return nil, fmt.Errorf("loading TLS cert %q: %w", cfg.CertFile, err)
		}
		creds = tc
		tlsConfig = transport.TLSConfig{CAFile: cfg.CertFile, ServerName: cfg.ServerName}
	} else {
		creds = credentials.NewTLS(&tls.Config{InsecureSkipVerify: true}) //nolint:gosec // opt-in for local clusters
		tlsConfig = transport.TLSConfig{Insecure: true}
	}

	callOptions := []grpc.CallOption{
		grpc.PerRPCCredentials(&client.CustomCredentials{
			TokenSource:         transport.NewCachedFileTokenSource(cfg.TokenFile),
			ImpersonationConfig: &transport.ImpersonationConfig{},
		}),
	}
	grpcOptions := []grpc.DialOption{
		grpc.WithDefaultCallOptions(callOptions...),
		grpc.WithTransportCredentials(creds),
	}
	gc, err := client.NewGRPCClient(cfg.ServerAddress, grpcOptions...)
	if err != nil {
		return nil, fmt.Errorf("creating gRPC client: %w", err)
	}

	restConfig := &transport.Config{TLS: tlsConfig, BearerTokenFile: cfg.TokenFile}
	rc, err := client.NewRESTClient(cfg.ServerAddress, client.WithConfig(restConfig))
	if err != nil {
		return nil, fmt.Errorf("creating REST client: %w", err)
	}

	return &APIClients{GRPC: gc, REST: rc}, nil
}
