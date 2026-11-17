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

package server

import (
	"context"
	"sync/atomic"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const restMaxRequestSize = 4 * 1024 * 1024

type directRESTRequestKey struct{}

type directRESTRequest struct {
	dispatched atomic.Bool
}

func checkRESTRequestSize(ctx context.Context, req proto.Message) error {
	request, ok := ctx.Value(directRESTRequestKey{}).(*directRESTRequest)
	if !ok {
		return nil // Native gRPC enforces its own transport limit.
	}
	request.dispatched.Store(true)
	if err := ctx.Err(); err != nil {
		return status.FromContextError(err).Err()
	}
	if size := proto.Size(req); size > restMaxRequestSize {
		return status.Errorf(codes.ResourceExhausted, "grpc: received message larger than max (%d vs. %d)", size, restMaxRequestSize)
	}
	return nil
}

// RESTError restores client-side cancellation outcomes for direct REST dispatch.
// Errors produced while decoding requests retain their original precedence.
// Native gRPC, plugin routes and the legacy Logs connection have no dispatch marker.
func RESTError(ctx context.Context, err error) error {
	request, ok := ctx.Value(directRESTRequestKey{}).(*directRESTRequest)
	if ok && request.dispatched.Load() && ctx.Err() != nil {
		return status.FromContextError(ctx.Err()).Err()
	}
	return err
}
