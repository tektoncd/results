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

package client

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/client-go/transport"
)

func TestGRPCClientClose(t *testing.T) {
	c, err := NewGRPCClient("https://localhost:1",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return nil, errors.New("dialing disabled in cleanup test")
		}),
	)
	if err != nil {
		t.Fatalf("NewGRPCClient() = %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if err := c.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	if got := c.(*grpcClient).conn.GetState(); got != connectivity.Shutdown {
		t.Errorf("connection state = %v, want Shutdown", got)
	}
}

func TestRESTClientCloseIdleConnections(t *testing.T) {
	tests := []struct {
		name string
		opts []RestOption
	}{
		{name: "default transport"},
		{
			name: "wrapped transport",
			opts: []RestOption{WithConfig(&transport.Config{BearerToken: "test-token"})},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var connections atomic.Int32
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"results":[]}`))
			}))
			server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
				if state == http.StateNew {
					connections.Add(1)
				}
			}
			server.Start()
			t.Cleanup(server.Close)
			c, err := NewRESTClient(server.URL, tt.opts...)
			if err != nil {
				t.Fatalf("NewRESTClient() = %v", err)
			}
			t.Cleanup(c.CloseIdleConnections)
			list := func() {
				t.Helper()
				if _, err := c.ListResults(context.Background(), &pb.ListResultsRequest{Parent: "default"}); err != nil {
					t.Fatalf("ListResults() = %v", err)
				}
			}
			list()
			list()
			if got := connections.Load(); got != 1 {
				t.Fatalf("connections before cleanup = %d, want 1 reused connection", got)
			}
			c.CloseIdleConnections()
			list()
			if got := connections.Load(); got != 2 {
				t.Errorf("connections after cleanup = %d, want a new connection", got)
			}
		})
	}
}
