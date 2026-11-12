package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tektoncd/results/pkg/api/server/config"
	"go.uber.org/zap"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"knative.dev/pkg/logging"
)

type gatewayPanicChecker struct{ called atomic.Bool }

func (c *gatewayPanicChecker) Check(context.Context, string, string, string) error {
	if !c.called.Swap(true) {
		panic("injected gateway failure")
	}
	return nil
}

func TestGatewayPanicRecovery(t *testing.T) {
	g := newGatewayFixture(t, false, nil, &gatewayPanicChecker{})
	func() {
		defer func() {
			if p := recover(); p != nil {
				t.Errorf("panic escaped REST handler: %v", p)
			}
		}()
		w := gatewayRequest(t, g.handler, "GET", resultsURL, nil, nil)
		body := gatewayJSON(t, w, 500)
		if body["code"] != float64(2) || body["message"] != "Error: injected gateway failure" {
			t.Fatalf("recovery contract: %v", body)
		}
	}()
	gatewayJSON(t, gatewayRequest(t, g.handler, "GET", resultsURL, nil, nil), 200)
	families, err := g.registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]float64{}
	for _, family := range families {
		if family.GetName() != "tekton_results_rest_requests_total" {
			continue
		}
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				if label.GetName() == "code" {
					counts[label.GetValue()] += metric.GetCounter().GetValue()
				}
			}
		}
	}
	if len(counts) != 2 || counts["500"] != 1 || counts["200"] != 1 {
		t.Fatalf("panic/success counts: %v", counts)
	}
	entries := g.logs.completions()
	if len(entries) != 2 || entries[0]["http.status_code"] != float64(500) || entries[1]["http.status_code"] != float64(200) {
		t.Fatalf("panic/success logs: %v", entries)
	}
	gatewayJSON(t, gatewayRequest(t, g.handler, "GET", "/healthz", nil, nil), 200)
}

func TestGatewayObservations(t *testing.T) {
	for _, histogram := range []bool{false, true} {
		g := newGatewayFixture(t, false, &config.Config{AUTH_DISABLE: true, PROMETHEUS_HISTOGRAM: histogram}, nil)
		g.health.SetServingStatus("stopped", healthpb.HealthCheckResponse_NOT_SERVING)
		gatewayJSON(t, gatewayRequest(t, g.handler, "GET", "/healthz", nil, nil), 200)
		gatewayJSON(t, gatewayRequest(t, g.handler, "GET", "/healthz?service=tekton.results.v1alpha2.Results", nil, nil), 200)
		gatewayJSON(t, gatewayRequest(t, g.handler, "GET", "/healthz?service=stopped", nil, nil), 503)
		gatewayJSON(t, gatewayRequest(t, g.handler, "GET", "/healthz?service=missing", nil, nil), 404)
		gatewayJSON(t, gatewayRequest(t, g.handler, "GET", resultsURL+"/missing-one", nil, nil), 404)
		gatewayJSON(t, gatewayRequest(t, g.handler, "GET", resultsURL+"/missing-two", nil, nil), 404)
		gatewayJSON(t, gatewayRequest(t, g.handler, "POST", resultsURL, []byte("{"), nil), 400)
		// Unmatched routes do not enter runtime gateway middleware.
		gatewayRequest(t, g.handler, "GET", "/unmatched", nil, nil)
		families, err := g.registry.Gather()
		if err != nil {
			t.Fatal(err)
		}
		var count float64
		var healthCount float64
		var healthSamples uint64
		var hasHistogram bool
		for _, family := range families {
			if family.GetName() == "tekton_results_rest_requests_total" {
				for _, metric := range family.Metric {
					count += metric.GetCounter().GetValue()
					for _, label := range metric.Label {
						if label.GetName() == "route" && label.GetValue() == "/healthz" {
							healthCount += metric.GetCounter().GetValue()
						}
						if strings.Contains(label.GetValue(), "missing-one") || strings.Contains(label.GetValue(), "missing-two") {
							t.Fatal("resource ID in metric label")
						}
					}
				}
			}
			if family.GetName() == "tekton_results_rest_request_duration_seconds" {
				hasHistogram = true
				for _, metric := range family.Metric {
					for _, label := range metric.Label {
						if label.GetName() == "route" && label.GetValue() == "/healthz" {
							healthSamples += metric.GetHistogram().GetSampleCount()
						}
					}
				}
			}
		}
		if count != 7 || healthCount != 4 || hasHistogram != histogram || histogram && healthSamples != 4 {
			t.Fatalf("count=%v healthCount=%v healthSamples=%v histogram=%v want=%v", count, healthCount, healthSamples, hasHistogram, histogram)
		}
		entries := g.logs.completions()
		if len(entries) != 5 {
			t.Fatalf("completion logs=%d", len(entries))
		}
		var healthLogs int
		for _, entry := range entries {
			fields := entry
			if fields["transport"] != "rest" || fields["grpc.user"] != "unknown" || fields["http.duration_ms"] == nil {
				t.Fatalf("log fields: %v", fields)
			}
			if fields["http.route"] == "/healthz" {
				healthLogs++
				code := fields["http.status_code"]
				if !(code == float64(404) && fields["level"] == "warn" || code == float64(503) && fields["level"] == "error") {
					t.Fatalf("unexpected health completion log: %v", fields)
				}
			}
		}
		if healthLogs != 2 {
			t.Fatalf("health failure logs=%d, want 2", healthLogs)
		}
	}
}

func TestGatewayIdentityLogging(t *testing.T) {
	g := newGatewayFixture(t, false, nil, nil)
	// Logging claims are deliberately unverified; auth remains service-owned.
	token := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1c2VyIiwiaXNzIjoiaXNzdWVyIn0.signature"
	// Use a syntactically valid signature encoding for ParseUnverified.
	token = strings.TrimSuffix(token, "signature") + "c2lnbmF0dXJl"
	gatewayJSON(t, gatewayRequest(t, g.handler, "GET", resultsURL, nil, http.Header{"Authorization": {"Bearer " + token}}), 200)
	entries := g.logs.completions()
	if len(entries) != 1 {
		t.Fatalf("logs=%d", len(entries))
	}
	fields := entries[0]
	if fields["grpc.user"] != "user" || fields["grpc.issuer"] != "issuer" || fields["grpc.auth_disabled"] != true {
		t.Fatalf("identity fields: %v", fields)
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), token) {
		t.Fatal("bearer token leaked into completion fields")
	}
}

func TestGatewayCommittedPanicAndFlush(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := newRESTMetrics(registry, false)
	if err != nil {
		t.Fatal(err)
	}
	mux := runtime.NewServeMux()
	ctx := logging.WithLogger(context.Background(), zap.NewNop().Sugar())
	runtime.WithMiddlewares(metrics.middleware(ctx, true), gatewayRecovery(mux))(mux)
	if err := mux.HandlePath("GET", "/committed", func(w http.ResponseWriter, _ *http.Request, _ map[string]string) {
		_, _ = w.Write([]byte("first"))
		if err := http.NewResponseController(w).Flush(); err != nil {
			panic(err)
		}
		panic("after first chunk")
	}); err != nil {
		t.Fatal(err)
	}
	w := gatewayRequest(t, mux, "GET", "/committed", nil, nil)
	if w.Code != 200 || w.Body.String() != "first" || !w.Flushed {
		t.Fatalf("committed response changed: %d %s", w.Code, w.Body)
	}
	recorder := httptest.NewRecorder()
	writer := &restResponseWriter{ResponseWriter: recorder}
	if err := http.NewResponseController(writer).Flush(); err != nil {
		t.Fatal(err)
	}
	if writer.status != 200 || !recorder.Flushed || writer.Unwrap() != recorder {
		t.Fatal("flush/unwrap/status semantics changed")
	}
}
