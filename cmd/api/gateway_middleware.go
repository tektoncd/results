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
	"strconv"
	"time"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
	"knative.dev/pkg/logging"
)

type restMetrics struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

func newRESTMetrics(reg prometheus.Registerer, histogram bool) (*restMetrics, error) {
	labels := []string{"method", "route", "code"}
	m := &restMetrics{requests: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tekton_results_rest_requests_total", Help: "Completed requests to matched Results REST gateway routes."}, labels)}
	if err := reg.Register(m.requests); err != nil {
		return nil, fmt.Errorf("registering REST requests: %w", err)
	}
	if histogram {
		m.duration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "tekton_results_rest_request_duration_seconds", Help: "Duration of completed requests to matched Results REST gateway routes."}, labels)
		if err := reg.Register(m.duration); err != nil {
			return nil, fmt.Errorf("registering REST duration: %w", err)
		}
	}
	return m, nil
}

func (m *restMetrics) middleware(base context.Context, authDisabled bool) runtime.Middleware {
	return func(next runtime.HandlerFunc) runtime.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request, params map[string]string) {
			start := time.Now()
			logger := logging.FromContext(base).Desugar().With(zap.Bool("grpc.auth_disabled", authDisabled))
			ctx := ctxzap.ToContext(r.Context(), logger)
			// Identity extraction enriches logs. Service methods remain the auth boundary.
			ctx, _ = determineAuth(metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", r.Header.Get("Authorization"))))
			ctx = logging.WithLogger(ctx, ctxzap.Extract(ctx).Sugar())
			r = r.WithContext(ctx)
			response := &restResponseWriter{ResponseWriter: w}
			route := "unknown"
			if pattern, ok := runtime.HTTPPattern(ctx); ok {
				route = pattern.String()
			}
			defer func() {
				code := response.status
				if code == 0 {
					code = http.StatusOK
				}
				duration := time.Since(start)
				labels := []string{r.Method, route, strconv.Itoa(code)}
				m.requests.WithLabelValues(labels...).Inc()
				if m.duration != nil {
					m.duration.WithLabelValues(labels...).Observe(duration.Seconds())
				}
				// Successful probes are frequent; retain their metrics without completion logs.
				if route == "/healthz" && code >= http.StatusOK && code < http.StatusMultipleChoices {
					return
				}
				log := logging.FromContext(ctx)
				fields := []any{"transport", "rest", "http.method", r.Method, "http.route", route, "http.status_code", code, "http.duration_ms", duration.Milliseconds()}
				switch {
				case code >= 500:
					log.Errorw("finished REST request", fields...)
				case code >= 400:
					log.Warnw("finished REST request", fields...)
				default:
					log.Infow("finished REST request", fields...)
				}
			}()
			next(response, r, params)
		}
	}
}

func gatewayRecovery(mux *runtime.ServeMux) runtime.Middleware {
	return func(next runtime.HandlerFunc) runtime.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request, params map[string]string) {
			defer func() {
				if p := recover(); p != nil {
					if response := w.(*restResponseWriter); response.status != 0 {
						logging.FromContext(r.Context()).Errorw("REST handler panic after response committed", "panic", p)
						return
					}
					_, marshaler := runtime.MarshalerForRequest(mux, r)
					runtime.HTTPError(r.Context(), mux, marshaler, w, r, recoveryHandler(p))
				}
			}()
			next(w, r, params)
		}
	}
}

type restResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *restResponseWriter) WriteHeader(code int) {
	if w.status != 0 {
		return
	}
	if code >= 200 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}
func (w *restResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(p)
}
func (w *restResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *restResponseWriter) FlushError() error {
	err := http.NewResponseController(w.ResponseWriter).Flush()
	if err == nil && w.status == 0 {
		w.status = http.StatusOK
	}
	return err
}
func (w *restResponseWriter) Flush() { _ = w.FlushError() }
