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
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"

	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"github.com/tektoncd/results/test/performance/report"
)

type queryResultsServer struct {
	pb.UnimplementedResultsServer
}

func (queryResultsServer) ListResults(_ context.Context, req *pb.ListResultsRequest) (*pb.ListResultsResponse, error) {
	resp := &pb.ListResultsResponse{}
	if req.GetPageToken() == "" {
		resp.NextPageToken = "next"
	}
	return resp, nil
}

func (queryResultsServer) ListRecords(_ context.Context, req *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error) {
	resp := &pb.ListRecordsResponse{}
	if req.GetPageToken() == "" {
		resp.NextPageToken = "next"
	}
	return resp, nil
}

func TestQueryCommandCompletion(t *testing.T) {
	for _, tt := range []struct {
		transport   string
		count       int
		concurrency int
		duration    int
	}{
		{transport: "grpc", count: 11, concurrency: 3},
		{transport: "rest", count: 11, concurrency: 3},
		{transport: "both", count: 10, concurrency: 3},
		{transport: "both", count: 3, concurrency: 8},
		{transport: "both", count: 1, concurrency: 3, duration: 1},
	} {
		t.Run(tt.transport+"/"+strconv.Itoa(tt.count), func(t *testing.T) {
			api := queryResultsServer{}
			gc := grpc.NewServer()
			pb.RegisterResultsServer(gc, api)
			t.Cleanup(gc.Stop)
			rc := runtime.NewServeMux()
			if err := pb.RegisterResultsHandlerServer(context.Background(), rc, api); err != nil {
				t.Fatal(err)
			}
			var grpcCalls, restCalls atomic.Int64
			https := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
					grpcCalls.Add(1)
					gc.ServeHTTP(w, r)
				} else {
					restCalls.Add(1)
					rc.ServeHTTP(w, r)
				}
			}))
			https.EnableHTTP2 = true
			https.StartTLS()
			t.Cleanup(https.Close)

			dir := t.TempDir()
			certPath := filepath.Join(dir, "ca.pem")
			cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: https.Certificate().Raw})
			if err := os.WriteFile(certPath, cert, 0o600); err != nil {
				t.Fatal(err)
			}
			tokenPath := filepath.Join(dir, "token")
			if err := os.WriteFile(tokenPath, []byte("test-token"), 0o600); err != nil {
				t.Fatal(err)
			}
			outputPath := filepath.Join(dir, "query.json")
			cmd := newRootCmd()
			args := []string{
				"query", "--server-addr", https.URL, "--server-name", "127.0.0.1",
				"--cert", certPath, "--token", tokenPath, "--output", outputPath,
				"--transport", tt.transport, "--count", strconv.Itoa(tt.count),
				"--concurrency", strconv.Itoa(tt.concurrency),
			}
			if tt.duration > 0 {
				args = append(args, "--duration", strconv.Itoa(tt.duration))
			}
			cmd.SetArgs(args)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			stdout, err := captureStdout(t, func() error { return cmd.ExecuteContext(ctx) })
			if err != nil {
				t.Fatal(err)
			}
			if ctx.Err() != nil {
				t.Fatal("query required external cancellation instead of completing its count")
			}
			data, err := os.ReadFile(outputPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(stdout) != string(data) {
				t.Error("stdout does not match the saved query report")
			}
			var got report.Report
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			if tt.duration > 0 {
				if got.Metrics.TotalOps <= int64(2*tt.count) {
					t.Errorf("timed query stopped at the count limit: %d page requests", got.Metrics.TotalOps)
				}
				if got.Meta.DurationMS < int64(tt.duration*1000) {
					t.Errorf("timed query ran for %d ms, want at least %d ms", got.Meta.DurationMS, tt.duration*1000)
				}
				return
			}
			if got.Metrics.TotalOps != int64(2*tt.count) || got.Metrics.TotalErrors != 0 {
				t.Errorf("page requests/errors = %d/%d, want %d/0", got.Metrics.TotalOps, got.Metrics.TotalErrors, 2*tt.count)
			}
			wantGRPC, wantREST := int64(2*tt.count), int64(0)
			switch tt.transport {
			case "rest":
				wantGRPC, wantREST = 0, int64(2*tt.count)
			case "both":
				wantGRPC, wantREST = int64(2*((tt.count+1)/2)), int64(2*(tt.count/2))
			}
			if grpcCalls.Load() != wantGRPC || restCalls.Load() != wantREST {
				t.Errorf("gRPC/REST requests = %d/%d, want %d/%d", grpcCalls.Load(), restCalls.Load(), wantGRPC, wantREST)
			}
			if tt.count >= len(defaultQueries()) && len(got.Metrics.ByOp) != len(defaultQueries()) {
				t.Errorf("ran %d query types, want %d", len(got.Metrics.ByOp), len(defaultQueries()))
			}
		})
	}
}
