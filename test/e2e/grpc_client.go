// Copyright 2022 The Tekton Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package e2e

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	"google.golang.org/grpc/credentials/oauth"
	"k8s.io/client-go/transport"

	resultsv1alpha2 "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	certFile         = "tekton-results-cert.pem"
	apiServerName    = "tekton-results-api-service.tekton-pipelines.svc.cluster.local"
	apiServerAddress = "localhost:50051"
	defCertFolder    = "/tmp/tekton-results/ssl"
)

var (
	certPath string
)

func init() {
	certFolder := os.Getenv("SSL_CERT_PATH")
	if len(certFolder) == 0 {
		certFolder = defCertFolder
	}
	certPath = path.Join(certFolder, certFile)
}

// newResultsClient creates a new Results GRPC client to talk to the Results API
// server.
func newResultsClient(t *testing.T, accessTokenPath string) resultsv1alpha2.ResultsClient {
	t.Helper()

	certPool, err := loadCertificates()
	if err != nil {
		t.Fatal(err)
	}

	conn, err := openConnection(certPool, accessTokenPath)
	if err != nil {
		t.Fatalf("Error connecting to the Results API server: %v", err)
	}

	return resultsv1alpha2.NewResultsClient(conn)
}

func loadCertificates() (*x509.CertPool, error) {
	certPool, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("error loading system cert pool: %w", err)
	}

	cert, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}

	if !certPool.AppendCertsFromPEM(cert) {
		return nil, errors.New("unable to append certificate to cert pool")
	}

	return certPool, nil
}

func openConnection(certPool *x509.CertPool, accessTokenPath string) (*grpc.ClientConn, error) {
	transportCredentials := credentials.NewClientTLSFromCert(certPool, apiServerName)
	tokenSource := transport.NewCachedFileTokenSource(accessTokenPath)
	opts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithDefaultCallOptions(grpc.PerRPCCredentials(oauth.TokenSource{TokenSource: tokenSource})),
		grpc.WithTransportCredentials(transportCredentials),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return grpc.DialContext(ctx, apiServerAddress, opts...)
}
