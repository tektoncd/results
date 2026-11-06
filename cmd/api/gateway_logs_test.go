package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tektoncd/results/pkg/api/server/config"
	"github.com/tektoncd/results/pkg/apis/v1alpha3"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/genproto/googleapis/api/httpbody"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func seedGatewayLog(t *testing.T, g *gatewayFixture, path string, size int) {
	t.Helper()
	ctx := context.Background()
	if _, err := g.server.CreateResult(ctx, &pb.CreateResultRequest{Parent: "foo", Result: &pb.Result{Name: "foo/results/log-result"}}); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(&v1alpha3.Log{Spec: v1alpha3.LogSpec{Type: v1alpha3.FileLogType, Resource: v1alpha3.Resource{Namespace: "foo", Name: "log-record"}}, Status: v1alpha3.LogStatus{Path: path, Size: int64(size), IsStored: true}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.server.CreateRecord(ctx, &pb.CreateRecordRequest{Parent: "foo/results/log-result", Record: &pb.Record{Name: "foo/results/log-result/records/log-record", Data: &pb.Any{Type: v1alpha3.LogRecordType, Value: data}}}); err != nil {
		t.Fatal(err)
	}
}

type gatewayFlushBarrier struct {
	http.ResponseWriter
	once    *sync.Once
	started chan struct{}
	release chan struct{}
}

func (w gatewayFlushBarrier) Flush() {
	w.ResponseWriter.(http.Flusher).Flush()
	w.once.Do(func() { close(w.started); <-w.release })
}

func TestGatewayLegacyLogs(t *testing.T) {
	g := newGatewayFixture(t, false, &config.Config{AUTH_DISABLE: true, LOGS_API: true, LOGS_TYPE: "File", LOGS_BUFFER_SIZE: 1024}, nil)
	content := strings.Repeat("known-log-content", 4096)
	path := filepath.Join(t.TempDir(), "log")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	seedGatewayLog(t, g, path, len(content))
	started, release, completed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(completed)
		g.handler.ServeHTTP(gatewayFlushBarrier{ResponseWriter: w, once: &once, started: started, release: release}, r)
	}))
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", h.URL+resultsURL+"/log-result/logs/log-record", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := h.Client().Do(req)
	if err != nil {
		close(release)
		t.Fatal(err)
	}
	defer resp.Body.Close()
	first := make([]byte, 100)
	if _, err := io.ReadFull(resp.Body, first); err != nil {
		close(release)
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		close(release)
		t.Fatal("no progressive flush")
	}
	select {
	case <-completed:
		close(release)
		t.Fatal("stream completed before first content was observed")
	default:
	}
	close(release)
	rest, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.ReplaceAll(string(append(first, rest...)), "\n", "")
	if resp.StatusCode != 200 || got != content {
		t.Fatalf("legacy stream status=%d bytes=%d", resp.StatusCode, len(got))
	}
	select {
	case <-completed:
	case <-ctx.Done():
		t.Fatal("legacy stream did not complete")
	}
	gatewayJSON(t, gatewayRequest(t, g.handler, "GET", resultsURL+"/log-result/logs", nil, nil), 200)
	gatewayJSON(t, gatewayRequest(t, g.handler, "DELETE", resultsURL+"/log-result/logs/log-record", nil, nil), 200)
	gatewayJSON(t, gatewayRequest(t, g.handler, "GET", resultsURL+"/log-result/logs/log-record", nil, nil), 404)
}

func TestGatewayLegacyBackendFailure(t *testing.T) {
	g := newGatewayFixture(t, false, &config.Config{AUTH_DISABLE: true, LOGS_API: true, LOGS_TYPE: "File"}, nil)
	seedGatewayLog(t, g, filepath.Join(t.TempDir(), "missing"), 1024)
	w := gatewayRequest(t, g.handler, "GET", resultsURL+"/log-result/logs/log-record", nil, nil)
	if w.Code != 500 {
		t.Fatalf("backend error status=%d body=%s", w.Code, w.Body)
	}
}

type gatewayFailingLogStream struct{ pb.UnimplementedLogsServer }

func (gatewayFailingLogStream) GetLog(_ *pb.GetLogRequest, stream pb.Logs_GetLogServer) error {
	if err := stream.Send(&httpbody.HttpBody{ContentType: "text/plain", Data: []byte("first backend chunk")}); err != nil {
		return err
	}
	return status.Error(codes.Internal, "backend failed after first chunk")
}

func TestGatewayLegacyCommittedBackendError(t *testing.T) {
	// Inject a mid-stream upstream failure over the retained legacy transport.
	g := newGatewayFixture(t, false, nil, nil)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gs := grpc.NewServer()
	pb.RegisterLogsServer(gs, gatewayFailingLogStream{})
	go func() { _ = gs.Serve(listener) }()
	t.Cleanup(gs.Stop)
	cfg := &config.Config{AUTH_DISABLE: true, LOGS_API: true, SERVER_PORT: strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	g.handler, err = newGateway(ctx, cfg, g.server, g.health, g.partial, []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithNoProxy()}, g.metrics)
	if err != nil {
		t.Fatal(err)
	}
	w := gatewayRequest(t, g.handler, "GET", resultsURL+"/log-result/logs/log-record", nil, nil)
	if w.Code != 200 || !w.Flushed || !strings.HasPrefix(w.Body.String(), "first backend chunk") || !strings.Contains(w.Body.String(), "backend failed after first chunk") {
		t.Fatalf("mid-stream failure changed: %d %s", w.Code, w.Body)
	}
	gatewayJSON(t, gatewayRequest(t, g.handler, "GET", resultsURL, nil, nil), 200)
}

func TestGatewayDisabledLogs(t *testing.T) {
	g := newGatewayFixture(t, false, nil, nil)
	w := gatewayRequest(t, g.handler, "GET", resultsURL+"/log-result/logs/log-record", nil, nil)
	if w.Code != 404 {
		t.Fatalf("disabled logs acquired a route: %d", w.Code)
	}
	gatewayJSON(t, gatewayRequest(t, g.handler, "GET", resultsURL, nil, nil), 200)
}

func TestGatewayLegacyCancellation(t *testing.T) {
	g := newGatewayFixture(t, false, &config.Config{AUTH_DISABLE: true, LOGS_API: true, LOGS_TYPE: "File", LOGS_BUFFER_SIZE: 1024}, nil)
	content := strings.Repeat("known-log-content", 4096)
	path := filepath.Join(t.TempDir(), "log")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	seedGatewayLog(t, g, path, len(content))
	started, release, completed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(completed)
		g.handler.ServeHTTP(gatewayFlushBarrier{ResponseWriter: w, once: &once, started: started, release: release}, r)
	}))
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", h.URL+resultsURL+"/log-result/logs/log-record", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := h.Client().Do(req)
	if err != nil {
		close(release)
		t.Fatal(err)
	}
	cancel()
	resp.Body.Close()
	close(release)
	select {
	case <-completed:
	case <-time.After(10 * time.Second):
		t.Fatal("disconnected legacy stream did not finish")
	}
	entries := g.logs.completions()
	if len(entries) != 1 || entries[0]["http.status_code"] != float64(200) {
		t.Fatalf("committed cancellation observations: %v", entries)
	}
	gatewayJSON(t, gatewayRequest(t, g.handler, "GET", resultsURL, nil, nil), 200)
}

func TestGatewayLegacyNotice(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		cfg := &config.Config{AUTH_DISABLE: true}
		if legacy {
			cfg.LOGS_API, cfg.LOGS_TYPE = true, "File"
		}
		g := newGatewayFixture(t, false, cfg, nil)
		var notices int
		for _, entry := range g.logs.entries {
			if strings.Contains(entry["msg"].(string), "Deprecated legacy Logs REST API") {
				notices++
			}
		}
		if (notices == 1) != legacy || notices > 1 {
			t.Fatalf("legacy=%v notices=%d", legacy, notices)
		}
	}
}

type gatewayDeniedChecker struct{}

func (gatewayDeniedChecker) Check(context.Context, string, string, string) error {
	return status.Error(codes.Unauthenticated, "denied by checker")
}

func TestGatewayPluginLogs(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "query_range") {
			t.Errorf("backend path=%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"success","data":{"result":[{"stream":{},"values":[["1625081600000000001","known plugin content"]]}]}}`)
	}))
	defer backend.Close()
	token := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(token, []byte("backend-token"), 0600); err != nil {
		t.Fatal(err)
	}
	g := newGatewayFixture(t, false, &config.Config{AUTH_DISABLE: true, LOGS_API: true, LOGS_TYPE: "Loki", LOGGING_PLUGIN_API_URL: backend.URL, LOGGING_PLUGIN_TOKEN_PATH: token, LOGGING_PLUGIN_QUERY_LIMIT: 10, LOGGING_PLUGIN_NAMESPACE_KEY: "namespace"}, nil)
	if !g.server.LogPluginServer.IsLogPluginEnabled {
		t.Fatal("fixture did not enable plugin")
	}
	_, err := g.server.CreateResult(context.Background(), &pb.CreateResultRequest{Parent: "foo", Result: &pb.Result{Name: "foo/results/log-result"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.server.CreateRecord(context.Background(), &pb.CreateRecordRequest{Parent: "foo/results/log-result", Record: &pb.Record{Name: "foo/results/log-result/records/log-record", Data: &pb.Any{Type: "tekton.dev/v1.PipelineRun", Value: []byte(`{"metadata":{"uid":"run-uid"},"status":{"startTime":"2026-10-01T00:00:00Z","completionTime":"2026-10-01T00:01:00Z"}}`)}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"v1alpha2", "v1alpha3"} {
		w := gatewayRequest(t, g.handler, "GET", "/apis/results.tekton.dev/"+version+"/parents/foo/results/log-result/logs/log-record", nil, nil)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "known plugin content") {
			t.Fatalf("plugin %s: %d %s", version, w.Code, w.Body)
		}
	}
	families, err := g.registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	if len(families) != 0 || len(g.logs.completions()) != 0 {
		t.Fatal("plugin requests entered gateway observations")
	}
	denied := newGatewayFixture(t, false, &config.Config{LOGS_API: true, LOGS_TYPE: "Loki", LOGGING_PLUGIN_API_URL: backend.URL, LOGGING_PLUGIN_TOKEN_PATH: token}, gatewayDeniedChecker{})
	w := gatewayRequest(t, denied.handler, "GET", "/apis/results.tekton.dev/v1alpha2/parents/foo/results/log-result/logs/log-record", nil, nil)
	if w.Code != 401 {
		t.Fatalf("plugin denial: %d %s", w.Code, w.Body)
	}
	gatewayJSON(t, gatewayRequest(t, g.handler, "GET", resultsURL, nil, nil), 200)
	gatewayJSON(t, gatewayRequest(t, g.handler, "GET", "/healthz", nil, nil), 200)
}
