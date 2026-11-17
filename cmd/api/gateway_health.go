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
	"sync/atomic"

	"github.com/tektoncd/results/internal/fieldmask"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// localHealthClient keeps the runtime health HTTP handler while removing its self connection.
type localHealthClient struct {
	server  healthpb.HealthServer
	partial *atomic.Bool
}

func (c localHealthClient) Check(ctx context.Context, req *healthpb.HealthCheckRequest, _ ...grpc.CallOption) (*healthpb.HealthCheckResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	resp, err := c.server.Check(ctx, req)
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, status.FromContextError(contextErr).Err()
	}
	if err != nil {
		return nil, err
	}
	if c.partial.Load() {
		md, _ := metadata.FromOutgoingContext(ctx)
		fieldmask.FromMetadata(md).Filter(resp)
	}
	return resp, nil
}

func (c localHealthClient) List(ctx context.Context, req *healthpb.HealthListRequest, _ ...grpc.CallOption) (*healthpb.HealthListResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	resp, err := c.server.List(ctx, req)
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, status.FromContextError(contextErr).Err()
	}
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (localHealthClient) Watch(context.Context, *healthpb.HealthCheckRequest, ...grpc.CallOption) (healthpb.Health_WatchClient, error) {
	return nil, status.Error(codes.Unimplemented, "REST health does not use Watch")
}
