package plugin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gocloud.dev/gcerrors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tektoncd/results/pkg/api/server/config"
	"github.com/tektoncd/results/pkg/api/server/db"
	"github.com/tektoncd/results/pkg/api/server/logger"
	"github.com/tektoncd/results/pkg/api/server/test"
	server "github.com/tektoncd/results/pkg/api/server/v1alpha2"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/plugin"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	"github.com/tektoncd/results/pkg/internal/jsonutil"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// fakeNetError is a minimal net.Error used to exercise the timeout branch of
// transportErrorToCode without opening real sockets.
type fakeNetError struct{ timeout bool }

func (e fakeNetError) Error() string   { return "fake net error" }
func (e fakeNetError) Timeout() bool   { return e.timeout }
func (e fakeNetError) Temporary() bool { return false }

// --- Pure mapping tests -----------------------------------------------------

func TestHTTPStatusToCode(t *testing.T) {
	cases := []struct {
		in   int
		want codes.Code
	}{
		{http.StatusBadRequest, codes.Internal}, // server-built query, not caller input
		{http.StatusUnauthorized, codes.Unauthenticated},
		{http.StatusForbidden, codes.PermissionDenied},
		{http.StatusNotFound, codes.NotFound},
		{http.StatusTooManyRequests, codes.ResourceExhausted},
		{http.StatusBadGateway, codes.Unavailable},
		{http.StatusServiceUnavailable, codes.Unavailable},
		{http.StatusGatewayTimeout, codes.Unavailable},
		{http.StatusInternalServerError, codes.Internal},
		{http.StatusNotImplemented, codes.Internal},
		{http.StatusTeapot, codes.Internal},
		{http.StatusOK, codes.Internal},
	}
	for _, tc := range cases {
		if got := plugin.HTTPStatusToCode(tc.in); got != tc.want {
			t.Errorf("HTTPStatusToCode(%d) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestCodeToHTTPStatus(t *testing.T) {
	cases := []struct {
		in   codes.Code
		want int
	}{
		{codes.OK, http.StatusOK},
		{codes.InvalidArgument, http.StatusBadRequest},
		{codes.FailedPrecondition, http.StatusBadRequest},
		{codes.Unauthenticated, http.StatusUnauthorized},
		{codes.PermissionDenied, http.StatusForbidden},
		{codes.NotFound, http.StatusNotFound},
		{codes.ResourceExhausted, http.StatusTooManyRequests},
		{codes.Unimplemented, http.StatusNotImplemented},
		{codes.Unavailable, http.StatusServiceUnavailable},
		{codes.DeadlineExceeded, http.StatusGatewayTimeout},
		{codes.Internal, http.StatusInternalServerError},
		{codes.Unknown, http.StatusInternalServerError},
		{codes.Aborted, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		if got := plugin.CodeToHTTPStatus(tc.in); got != tc.want {
			t.Errorf("CodeToHTTPStatus(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestTransportErrorToCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"timeout net error", fakeNetError{timeout: true}, codes.DeadlineExceeded},
		{"context deadline exceeded", context.DeadlineExceeded, codes.DeadlineExceeded},
		{"non-timeout net error", fakeNetError{timeout: false}, codes.Unavailable},
		{"plain error", errors.New("connection refused"), codes.Unavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := plugin.TransportErrorToCode(tc.err); got != tc.want {
				t.Errorf("TransportErrorToCode() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBlobCodeToGRPC(t *testing.T) {
	cases := []struct {
		in   gcerrors.ErrorCode
		want codes.Code
	}{
		{gcerrors.NotFound, codes.NotFound},
		{gcerrors.PermissionDenied, codes.PermissionDenied},
		{gcerrors.InvalidArgument, codes.Internal}, // server-built bucket URL and keys
		{gcerrors.ResourceExhausted, codes.ResourceExhausted},
		{gcerrors.DeadlineExceeded, codes.DeadlineExceeded},
		{gcerrors.FailedPrecondition, codes.FailedPrecondition},
		{gcerrors.Unimplemented, codes.Unimplemented},
		{gcerrors.OK, codes.Internal},
		{gcerrors.Unknown, codes.Internal},
		{gcerrors.Internal, codes.Internal},
		{gcerrors.AlreadyExists, codes.Internal},
		{gcerrors.Canceled, codes.Internal},
	}
	for _, tc := range cases {
		if got := plugin.BlobCodeToGRPC(tc.in); got != tc.want {
			t.Errorf("BlobCodeToGRPC(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// --- Helpers for backend end-to-end error-code tests ------------------------

func newLokiErrServer(t *testing.T, apiURL string) *server.Server {
	t.Helper()
	tokenDir := t.TempDir()
	tokenPath := filepath.Join(tokenDir, "token")
	if err := os.WriteFile(tokenPath, []byte("dummytoken"), 0600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	srv, err := server.New(&config.Config{
		LOGS_API:                                true,
		LOGS_TYPE:                               "Loki",
		DB_ENABLE_AUTO_MIGRATION:                true,
		LOGGING_PLUGIN_TOKEN_PATH:               tokenPath,
		LOGGING_PLUGIN_API_URL:                  apiURL,
		LOGGING_PLUGIN_TLS_VERIFICATION_DISABLE: true,
		LOGGING_PLUGIN_NAMESPACE_KEY:            "kubernetes.namespace_name",
		LOGGING_PLUGIN_CONTAINER_KEY:            "kubernetes.container_name",
		LOGGING_PLUGIN_QUERY_LIMIT:              1500,
		LOGGING_PLUGIN_MULTIPART_REGEX:          "",
	}, logger.Get("info"), test.NewDB(t))
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return srv
}

func newSplunkErrServer(t *testing.T, apiURL string) *server.Server {
	t.Helper()
	srv, err := server.New(&config.Config{
		LOGS_API:                                true,
		LOGS_TYPE:                               "Splunk",
		DB_ENABLE_AUTO_MIGRATION:                true,
		LOGGING_PLUGIN_API_URL:                  apiURL,
		LOGGING_PLUGIN_NAMESPACE_KEY:            "kubernetes.namespace_name",
		LOGGING_PLUGIN_CONTAINER_KEY:            "kubernetes.container_name",
		LOGGING_PLUGIN_QUERY_PARAMS:             "index=konflux",
		LOGGING_PLUGIN_TLS_VERIFICATION_DISABLE: true,
	}, logger.Get("info"), test.NewDB(t))
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return srv
}

func pipelineRunErrRecord(t *testing.T) *db.Record {
	t.Helper()
	return &db.Record{
		Name: "8a0ee8cc-ad65-4b0d-afb3-12149028cab5",
		Type: "tekton.dev/v1.PipelineRun",
		Data: jsonutil.AnyBytes(t, pipelinev1.PipelineRun{
			Status: pipelinev1.PipelineRunStatus{
				PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
					StartTime:      &metav1.Time{Time: time.Now().Add(-time.Minute)},
					CompletionTime: &metav1.Time{Time: time.Now()},
				},
			},
		}),
	}
}

func taskRunErrRecord(t *testing.T) *db.Record {
	t.Helper()
	return &db.Record{
		Name: "25274ae9-d521-4a9c-b254-122c17f64941",
		Type: "tekton.dev/v1.TaskRun",
		Data: jsonutil.AnyBytes(t, pipelinev1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{UID: "25274ae9-d521-4a9c-b254-122c17f64941"},
			Status: pipelinev1.TaskRunStatus{
				TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
					StartTime:      &metav1.Time{Time: time.Now().Add(-time.Minute)},
					CompletionTime: &metav1.Time{Time: time.Now()},
				},
			},
		}),
	}
}

// --- Loki backend end-to-end error-code tests -------------------------------

func TestGetLokiLogs_ErrorCodes(t *testing.T) {
	cases := []struct {
		httpStatus int
		want       codes.Code
	}{
		{http.StatusBadRequest, codes.Internal}, // server-built query, not caller input
		{http.StatusUnauthorized, codes.Unauthenticated},
		{http.StatusForbidden, codes.PermissionDenied},
		{http.StatusNotFound, codes.NotFound},
		{http.StatusTooManyRequests, codes.ResourceExhausted},
		{http.StatusBadGateway, codes.Unavailable},
		{http.StatusServiceUnavailable, codes.Unavailable},
		{http.StatusGatewayTimeout, codes.Unavailable},
		{http.StatusInternalServerError, codes.Internal},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("http_%d", tc.httpStatus), func(t *testing.T) {
			mock := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.httpStatus)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "error", "error": "backend failure"})
			}))
			defer mock.Close()

			srv := newLokiErrServer(t, mock.URL)
			var buf bytes.Buffer
			err := plugin.GetLokiLogs(srv.LogPluginServer, &buf, "tekton-loki-predev", pipelineRunErrRecord(t))
			if err == nil {
				t.Fatalf("expected error for HTTP %d", tc.httpStatus)
			}
			if got := status.Code(err); got != tc.want {
				t.Fatalf("HTTP %d: got code %v, want %v (err: %v)", tc.httpStatus, got, tc.want, err)
			}
		})
	}
}

func TestGetLokiLogs_TransportError(t *testing.T) {
	mock := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	apiURL := mock.URL
	mock.Close() // connection now refused

	srv := newLokiErrServer(t, apiURL)
	var buf bytes.Buffer
	err := plugin.GetLokiLogs(srv.LogPluginServer, &buf, "tekton-loki-predev", pipelineRunErrRecord(t))
	if err == nil {
		t.Fatal("expected transport error")
	}
	if got := status.Code(err); got != codes.Unavailable {
		t.Fatalf("got %v, want Unavailable (err: %v)", got, err)
	}
}

// --- Splunk backend end-to-end error-code tests -----------------------------

func TestGetSplunkLogs_ErrorCodes(t *testing.T) {
	t.Setenv("SPLUNK_SEARCH_TOKEN", "dummy-token")
	cases := []struct {
		httpStatus int
		want       codes.Code
	}{
		{http.StatusBadRequest, codes.Internal}, // server-built query, not caller input
		{http.StatusUnauthorized, codes.Unauthenticated},
		{http.StatusForbidden, codes.PermissionDenied},
		{http.StatusNotFound, codes.NotFound},
		{http.StatusTooManyRequests, codes.ResourceExhausted},
		{http.StatusServiceUnavailable, codes.Unavailable},
		{http.StatusInternalServerError, codes.Internal},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("http_%d", tc.httpStatus), func(t *testing.T) {
			mock := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/services/search/v2/jobs" {
					w.WriteHeader(tc.httpStatus)
					return
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer mock.Close()

			srv := newSplunkErrServer(t, mock.URL)
			var buf bytes.Buffer
			err := plugin.GetSplunkLogs(srv.LogPluginServer, &buf, "rh-acs-tenant", taskRunErrRecord(t))
			if err == nil {
				t.Fatalf("expected error for HTTP %d", tc.httpStatus)
			}
			if got := status.Code(err); got != tc.want {
				t.Fatalf("HTTP %d: got code %v, want %v (err: %v)", tc.httpStatus, got, tc.want, err)
			}
		})
	}
}

func TestGetSplunkLogs_TransportError(t *testing.T) {
	t.Setenv("SPLUNK_SEARCH_TOKEN", "dummy-token")
	mock := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	apiURL := mock.URL
	mock.Close() // connection now refused

	srv := newSplunkErrServer(t, apiURL)
	var buf bytes.Buffer
	err := plugin.GetSplunkLogs(srv.LogPluginServer, &buf, "rh-acs-tenant", taskRunErrRecord(t))
	if err == nil {
		t.Fatal("expected transport error")
	}
	if got := status.Code(err); got != codes.Unavailable {
		t.Fatalf("got %v, want Unavailable (err: %v)", got, err)
	}
}

// --- Blob backend error mapping test ----------------------------------------

// TestGetBlobLogs_OpenBucketError_MapsToStatus verifies that a bucket-open
// failure is returned as a gRPC status error (not a bare error). An
// unregistered URL scheme yields a plain (non-gcerr) error, which maps to
// codes.Internal; the per-code mapping is covered by TestBlobCodeToGRPC.
func TestGetBlobLogs_OpenBucketError_MapsToStatus(t *testing.T) {
	srv, err := server.New(&config.Config{
		LOGS_API:                                true,
		LOGS_TYPE:                               "Blob",
		DB_ENABLE_AUTO_MIGRATION:                true,
		LOGGING_PLUGIN_API_URL:                  "no-such-scheme://bucket",
		LOGGING_PLUGIN_TLS_VERIFICATION_DISABLE: true,
		LOGGING_PLUGIN_MULTIPART_REGEX:          "",
	}, logger.Get("info"), test.NewDB(t))
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	var buf bytes.Buffer
	err = plugin.GetBlobLogs(srv.LogPluginServer, &buf, "foo", taskRunErrRecord(t))
	if err == nil {
		t.Fatal("expected error from unopenable bucket")
	}
	if got := status.Code(err); got != codes.Internal {
		t.Fatalf("got %v, want Internal (err: %v)", got, err)
	}
}

// --- LogMux end-to-end HTTP status tests ------------------------------------

func createLogMuxRecord(t *testing.T, srv *server.Server) {
	t.Helper()
	ctx := context.Background()
	res, err := srv.CreateResult(ctx, &pb.CreateResultRequest{
		Parent: "foo",
		Result: &pb.Result{Name: "foo/results/bar"},
	})
	if err != nil {
		t.Fatalf("CreateResult: %v", err)
	}
	_, err = srv.CreateRecord(ctx, &pb.CreateRecordRequest{
		Parent: res.GetName(),
		Record: &pb.Record{
			Name: record.FormatName(res.GetName(), "baz"),
			Data: &pb.Any{
				Type: "tekton.dev/v1.PipelineRun",
				Value: jsonutil.AnyBytes(t, pipelinev1.PipelineRun{
					Status: pipelinev1.PipelineRunStatus{
						PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
							StartTime:      &metav1.Time{Time: time.Now().Add(-time.Minute)},
							CompletionTime: &metav1.Time{Time: time.Now()},
						},
					},
				}),
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
}

func logMuxRequest() *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/logs", nil)
	req.SetPathValue("parent", "foo")
	req.SetPathValue("resultID", "bar")
	req.SetPathValue("recordID", "baz")
	return req
}

func TestLogMux_ErrorCodes(t *testing.T) {
	cases := []struct {
		backendStatus int
		wantHTTP      int
	}{
		{http.StatusBadRequest, http.StatusInternalServerError},
		{http.StatusUnauthorized, http.StatusUnauthorized},
		{http.StatusForbidden, http.StatusForbidden},
		{http.StatusNotFound, http.StatusNotFound},
		{http.StatusTooManyRequests, http.StatusTooManyRequests},
		{http.StatusServiceUnavailable, http.StatusServiceUnavailable},
		{http.StatusInternalServerError, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("backend_%d", tc.backendStatus), func(t *testing.T) {
			mock := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.backendStatus)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "error"})
			}))
			defer mock.Close()

			srv := newLokiErrServer(t, mock.URL)
			createLogMuxRecord(t, srv)

			rr := httptest.NewRecorder()
			srv.LogPluginServer.LogMux().ServeHTTP(rr, logMuxRequest())

			if rr.Code != tc.wantHTTP {
				t.Fatalf("backend HTTP %d: got status %d, want %d (body: %s)", tc.backendStatus, rr.Code, tc.wantHTTP, rr.Body.String())
			}
		})
	}
}

func TestLogMux_Success(t *testing.T) {
	mock := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"result": []map[string]interface{}{
					{"stream": map[string]string{}, "values": [][]string{{"1", "hello world"}}},
				},
			},
		})
	}))
	defer mock.Close()

	srv := newLokiErrServer(t, mock.URL)
	createLogMuxRecord(t, srv)

	rr := httptest.NewRecorder()
	srv.LogPluginServer.LogMux().ServeHTTP(rr, logMuxRequest())

	if rr.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "hello world") {
		t.Fatalf("body missing expected log content: %s", rr.Body.String())
	}
}
