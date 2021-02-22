/*
Copyright 2020 The Tekton Authors

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
	"context"
	"crypto/x509"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	creds "github.com/tektoncd/results/pkg/watcher/grpc"
	"github.com/tektoncd/results/pkg/watcher/reconciler"
	"github.com/tektoncd/results/pkg/watcher/reconciler/pipelinerun"
	"github.com/tektoncd/results/pkg/watcher/reconciler/taskrun"
	v1alpha2pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/transport"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/injection"
	"knative.dev/pkg/injection/sharedmain"
	"knative.dev/pkg/signals"
	_ "knative.dev/pkg/system/testing"
)

const (
	// Service Account token path. See https://kubernetes.io/docs/tasks/access-application-cluster/access-cluster/#accessing-the-api-from-a-pod
	podTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"
)

var (
	apiAddr                 = flag.String("api_addr", "localhost:50051", "Address of API server to report to")
	authMode                = flag.String("auth_mode", "", "Authentication mode to use when making requests. If not set, no additional credentials will be used in the request. Valid values: [google]")
	disableCRDUpdate        = flag.Bool("disable_crd_update", false, "Disables Tekton CRD annotation update on reconcile.")
	authToken               = flag.String("token", "", "Authentication token to use in requests. If not specified, on-cluster configuration is assumed.")
	completedRunGracePeriod = flag.Duration("completed_run_grace_period", 0, "Grace period duration before Runs should be deleted. If 0, Runs will not be deleted. If < 0, Runs will be deleted immediately.")
)

func main() {
	flag.Parse()
	// TODO: Enable leader election.
	ctx := sharedmain.WithHADisabled(signals.NewContext())

	conn, err := connectToAPIServer(ctx, *apiAddr, *authMode)
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	results := v1alpha2pb.NewResultsClient(conn)

	cfg := &reconciler.Config{
		DisableAnnotationUpdate:      *disableCRDUpdate,
		CompletedResourceGracePeriod: *completedRunGracePeriod,
	}

	restCfg := sharedmain.ParseAndGetConfigOrDie()
	sharedmain.MainWithConfig(injection.WithNamespaceScope(ctx, ""), "watcher", restCfg,
		func(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
			return pipelinerun.NewControllerWithConfig(ctx, results, cfg)
		}, func(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
			return taskrun.NewControllerWithConfig(ctx, results, cfg)
		},
	)
}

func connectToAPIServer(ctx context.Context, apiAddr string, authMode string) (*grpc.ClientConn, error) {
	// Load TLS certs
	certs, err := loadCerts()
	if err != nil {
		log.Fatalf("error loading cert pool: %v", err)
	}
	cred := credentials.NewClientTLSFromCert(certs, "")

	opts := []grpc.DialOption{
		grpc.WithBlock(),
	}
	// Add in additional credentials to requests if desired.
	switch authMode {
	case "google":
		opts = append(opts,
			grpc.WithAuthority(apiAddr),
			grpc.WithTransportCredentials(cred),
			grpc.WithDefaultCallOptions(grpc.PerRPCCredentials(creds.Google())),
		)
	case "token":
		var ts oauth2.TokenSource
		if t := *authToken; t != "" {
			ts = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: t})
		} else {
			ts = transport.NewCachedFileTokenSource(podTokenPath)
		}
		opts = append(opts,
			grpc.WithDefaultCallOptions(grpc.PerRPCCredentials(oauth.TokenSource{TokenSource: ts})),
			grpc.WithTransportCredentials(cred),
		)
	case "insecure":
		opts = append(opts, grpc.WithInsecure())
	}

	log.Printf("dialing %s...\n", apiAddr)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return grpc.DialContext(ctx, apiAddr, opts...)
}

func loadCerts() (*x509.CertPool, error) {
	// Setup TLS certs to the server.
	f, err := os.Open("/etc/tls/tls.crt")
	if err != nil {
		log.Println("no local cluster cert found, defaulting to system pool...")
		return x509.SystemCertPool()
	}
	defer f.Close()
	b, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("unable to read TLS cert file: %v", err)
	}

	certs, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("error loading cert pool: %v", err)
	}
	if ok := certs.AppendCertsFromPEM(b); !ok {
		return nil, fmt.Errorf("unable to add cert to pool")
	}
	return certs, nil
}
