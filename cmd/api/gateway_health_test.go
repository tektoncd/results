package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

func TestGatewayHealthWithoutLoopback(t *testing.T) {
	for _, baseline := range []bool{true, false} {
		g := newGatewayFixture(t, baseline, nil, nil)
		g.health.SetServingStatus("stopped", healthpb.HealthCheckResponse_NOT_SERVING)
		for _, tc := range []struct {
			path string
			code int
		}{
			{"/healthz", 200},
			{"/healthz?service=tekton.results.v1alpha2.Results", 200},
			{"/healthz?service=stopped", 503},
			{"/healthz?service=missing", 404},
		} {
			w := gatewayRequest(t, g.handler, http.MethodGet, tc.path, nil, nil)
			if w.Code != tc.code {
				t.Errorf("baseline=%v %s: got %d %s, want %d", baseline, tc.path, w.Code, w.Body, tc.code)
			}
		}
		g.partial.Store(true)
		w := gatewayRequest(t, g.handler, http.MethodGet, "/healthz?fields=missing", nil, nil)
		if w.Code != 503 {
			t.Errorf("baseline=%v filtered status: %d %s", baseline, w.Code, w.Body)
		}
		g.partial.Store(false)
		w = gatewayRequest(t, g.handler, http.MethodGet, "/healthz?fields=missing", nil, nil)
		if w.Code != 200 {
			t.Errorf("baseline=%v disabled fields: %d %s", baseline, w.Code, w.Body)
		}
	}
}

type cancelingHealthServer struct {
	healthpb.UnimplementedHealthServer
	cancel context.CancelFunc
}

func (s cancelingHealthServer) Check(context.Context, *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	s.cancel()
	return nil, status.Error(codes.NotFound, "unknown service")
}

func (s cancelingHealthServer) List(context.Context, *healthpb.HealthListRequest) (*healthpb.HealthListResponse, error) {
	s.cancel()
	return nil, status.Error(codes.Internal, "interrupted health lookup")
}

func TestLocalHealthCancellationDuringError(t *testing.T) {
	for _, list := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		client := localHealthClient{server: cancelingHealthServer{cancel: cancel}, partial: new(atomic.Bool)}
		var err error
		if list {
			_, err = client.List(ctx, &healthpb.HealthListRequest{})
		} else {
			_, err = client.Check(ctx, &healthpb.HealthCheckRequest{})
		}
		cancel()
		if status.Code(err) != codes.Canceled {
			t.Fatalf("list=%v canceled health lookup: %v", list, err)
		}
	}
}

func TestGatewayHealthContext(t *testing.T) {
	for _, baseline := range []bool{true, false} {
		g := newGatewayFixture(t, baseline, nil, nil)
		for _, expired := range []bool{false, true} {
			var ctx context.Context
			var cancel context.CancelFunc
			want := 499
			if expired {
				ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				want = 504
			} else {
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			}
			req := httptest.NewRequest(http.MethodGet, "/healthz", nil).WithContext(ctx)
			w := httptest.NewRecorder()
			g.handler.ServeHTTP(w, req)
			cancel()
			if w.Code != want {
				t.Errorf("baseline=%v expired=%v: %d %s, want %d", baseline, expired, w.Code, w.Body, want)
			}
		}
	}
}
