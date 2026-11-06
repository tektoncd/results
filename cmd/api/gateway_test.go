package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"encoding/json"
	"sync"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tektoncd/results/internal/fieldmask"
	"github.com/tektoncd/results/pkg/api/server/config"
	"github.com/tektoncd/results/pkg/api/server/test"
	v1alpha2 "github.com/tektoncd/results/pkg/api/server/v1alpha2"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth/impersonation"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/encoding/protojson"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"knative.dev/pkg/logging"
)

const resultsURL = "/apis/results.tekton.dev/v1alpha2/parents/foo/results"

type gatewayFixture struct {
	server   *v1alpha2.Server
	health   *health.Server
	partial  *atomic.Bool
	handler  http.Handler
	db       *gorm.DB
	registry *prometheus.Registry
	metrics  *restMetrics
	logs     *gatewayLogSink
}

func newGatewayFixture(t *testing.T, baseline bool, cfg *config.Config, checker auth.Checker) *gatewayFixture {
	t.Helper()
	if cfg == nil {
		cfg = &config.Config{AUTH_DISABLE: true}
	}
	cfg.DB_ENABLE_AUTO_MIGRATION = true
	cfg.SERVER_PORT = "0" // Direct requests must work without any gRPC listener.
	g := &gatewayFixture{health: health.NewServer(), partial: new(atomic.Bool), db: test.NewDB(t)}
	g.db.Logger = gormlogger.Discard
	g.registry = prometheus.NewRegistry()
	g.logs = &gatewayLogSink{}
	core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(g.logs), zap.DebugLevel)
	var err error
	g.metrics, err = newRESTMetrics(g.registry, cfg.PROMETHEUS_HISTOGRAM)
	if err != nil {
		t.Fatal(err)
	}
	if checker == nil {
		checker = auth.AllowAll{}
	}
	g.server, err = v1alpha2.New(cfg, zap.NewNop().Sugar(), g.db, v1alpha2.WithAuth(checker))
	if err != nil {
		t.Fatal(err)
	}
	g.health.SetServingStatus("tekton.results.v1alpha2.Results", healthpb.HealthCheckResponse_SERVING)
	ctx, cancel := context.WithCancel(logging.WithLogger(context.Background(), zap.New(core).Sugar()))
	t.Cleanup(cancel)
	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithNoProxy(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100 * 1024 * 1024))}
	if baseline || cfg.LOGS_API && !g.server.LogPluginServer.IsLogPluginEnabled {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		cfg.SERVER_PORT = strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
		gs := grpc.NewServer(grpc.ChainUnaryInterceptor(fieldmask.UnaryServerInterceptor(g.partial)))
		pb.RegisterResultsServer(gs, g.server)
		pb.RegisterLogsServer(gs, g.server)
		healthpb.RegisterHealthServer(gs, g.health)
		go func() {
			if err := gs.Serve(listener); err != nil {
				t.Logf("gRPC fixture stopped: %v", err)
			}
		}()
		t.Cleanup(gs.Stop)
	}
	conn, err := grpc.NewClient(":"+cfg.SERVER_PORT, dialOpts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if baseline {
		// Freeze the old transport for compatibility comparisons; production uses newGateway.
		opts := []runtime.ServeMuxOption{
			runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
				MarshalOptions:   protojson.MarshalOptions{UseProtoNames: true},
				UnmarshalOptions: protojson.UnmarshalOptions{DiscardUnknown: true},
			}),
			runtime.WithMetadata(fieldmask.MetadataAnnotator),
			runtime.WithHealthzEndpoint(healthpb.NewHealthClient(conn)),
		}
		if !cfg.AUTH_DISABLE && cfg.AUTH_IMPERSONATE {
			opts = append(opts, runtime.WithIncomingHeaderMatcher(impersonation.HeaderMatcher))
		}
		mux := runtime.NewServeMux(opts...)
		if err := pb.RegisterResultsHandlerFromEndpoint(ctx, mux, ":"+cfg.SERVER_PORT, dialOpts); err != nil {
			t.Fatal(err)
		}
		g.handler = v1alpha2.Handler(mux, g.server.LogPluginServer)
	} else {
		g.handler, err = newGateway(ctx, cfg, g.server, g.health, g.partial, dialOpts, g.metrics)
		if err != nil {
			t.Fatal(err)
		}
	}
	return g
}

func gatewayRequest(t *testing.T, handler http.Handler, method, path string, body []byte, headers http.Header) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	for k, values := range headers {
		req.Header[k] = values
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

// gatewayLogSink records JSON logs without adding a test dependency to vendor.
type gatewayLogSink struct {
	sync.Mutex
	entries []map[string]any
}

func (s *gatewayLogSink) Write(p []byte) (int, error) {
	var entry map[string]any
	if err := json.Unmarshal(p, &entry); err != nil {
		return 0, err
	}
	s.Lock()
	defer s.Unlock()
	s.entries = append(s.entries, entry)
	return len(p), nil
}
func (s *gatewayLogSink) completions() []map[string]any {
	s.Lock()
	defer s.Unlock()
	var entries []map[string]any
	for _, e := range s.entries {
		if e["msg"] == "finished REST request" {
			entries = append(entries, e)
		}
	}
	return entries
}
