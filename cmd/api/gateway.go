// Copyright 2026 The Tekton Authors
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

package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/tektoncd/results/internal/fieldmask"
	"github.com/tektoncd/results/pkg/api/server/config"
	v1alpha2 "github.com/tektoncd/results/pkg/api/server/v1alpha2"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth/impersonation"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"knative.dev/pkg/logging"
)

func newGateway(ctx context.Context, cfg *config.Config, results *v1alpha2.Server, healthServer healthpb.HealthServer, partial *atomic.Bool, dialOpts []grpc.DialOption, metrics *restMetrics) (http.Handler, error) {
	opts := []runtime.ServeMuxOption{
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions:   protojson.MarshalOptions{UseProtoNames: true},
			UnmarshalOptions: protojson.UnmarshalOptions{DiscardUnknown: true},
		}),
		runtime.WithMetadata(fieldmask.MetadataAnnotator),
		runtime.WithErrorHandler(func(ctx context.Context, mux *runtime.ServeMux, marshaler runtime.Marshaler, w http.ResponseWriter, r *http.Request, err error) {
			runtime.DefaultHTTPErrorHandler(ctx, mux, marshaler, w, r, v1alpha2.RESTError(ctx, err))
		}),
		runtime.WithForwardResponseOption(fieldmask.ForwardResponseFilter(partial)),
		runtime.WithForwardResponseOption(gatewayResponseLimit),
	}
	if !cfg.AUTH_DISABLE && cfg.AUTH_IMPERSONATE {
		opts = append(opts, runtime.WithIncomingHeaderMatcher(impersonation.HeaderMatcher))
	}
	opts = append(opts,
		func(mux *runtime.ServeMux) {
			runtime.WithMiddlewares(metrics.middleware(ctx, cfg.AUTH_DISABLE), gatewayRecovery(mux))(mux)
		},
		runtime.WithHealthzEndpoint(localHealthClient{server: healthServer, partial: partial}),
	)
	mux := runtime.NewServeMux(opts...)
	endpoint := ":" + cfg.SERVER_PORT
	if err := pb.RegisterResultsHandlerServer(ctx, mux, results); err != nil {
		return nil, fmt.Errorf("registering Results gateway: %w", err)
	}
	if cfg.LOGS_API && !results.LogPluginServer.IsLogPluginEnabled {
		logging.FromContext(ctx).Warn("Deprecated legacy Logs REST API retains a loopback gRPC connection; use a logging plugin")
		if err := pb.RegisterLogsHandlerFromEndpoint(ctx, mux, endpoint, dialOpts); err != nil {
			return nil, fmt.Errorf("registering legacy Logs gateway: %w", err)
		}
	}
	return v1alpha2.Handler(mux, results.LogPluginServer), nil
}

const restMaxResponseSize = 100 * 1024 * 1024

func gatewayResponseLimit(ctx context.Context, _ http.ResponseWriter, resp proto.Message) error {
	method, _ := runtime.RPCMethod(ctx)
	if resp == nil || !strings.HasPrefix(method, "/tekton.results.v1alpha2.Results/") {
		return nil
	}
	if err := v1alpha2.RESTError(ctx, nil); err != nil {
		return err
	}
	if size := proto.Size(resp); size > restMaxResponseSize {
		return status.Errorf(codes.ResourceExhausted, "grpc: received message larger than max (%d vs. %d)", size, restMaxResponseSize)
	}
	return nil
}
