package plugin_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	server "github.com/tektoncd/results/pkg/api/server/v1alpha2"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/log"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	"github.com/tektoncd/results/pkg/internal/jsonutil"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	pb3 "github.com/tektoncd/results/proto/v1alpha3/results_go_proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// createGetLogRecord stores a PipelineRun record and returns the log name to
// pass to GetLog. When withTimes is false the record has no start/completion
// time, which makes the backend plugin fail with a plain (non-status) error.
func createGetLogRecord(t *testing.T, srv *server.Server, withTimes bool) string {
	t.Helper()
	ctx := context.Background()

	res, err := srv.CreateResult(ctx, &pb.CreateResultRequest{
		Parent: "foo",
		Result: &pb.Result{Name: "foo/results/bar"},
	})
	if err != nil {
		t.Fatalf("CreateResult: %v", err)
	}

	pr := pipelinev1.PipelineRun{}
	if withTimes {
		pr.Status.StartTime = &metav1.Time{Time: time.Now().Add(-time.Minute)}
		pr.Status.CompletionTime = &metav1.Time{Time: time.Now()}
	}

	if _, err := srv.CreateRecord(ctx, &pb.CreateRecordRequest{
		Parent: res.GetName(),
		Record: &pb.Record{
			Name: record.FormatName(res.GetName(), "baz"),
			Data: &pb.Any{
				Type:  "tekton.dev/v1.PipelineRun",
				Value: jsonutil.AnyBytes(t, pr),
			},
		},
	}); err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}

	return log.FormatName(res.GetName(), "baz")
}

// TestGetLog_PropagatesBackendErrorCodes verifies that the gRPC streaming
// GetLog surfaces the log backend's failure to the client instead of logging it
// and returning an empty, successful stream.
func TestGetLog_PropagatesBackendErrorCodes(t *testing.T) {
	cases := []struct {
		backendStatus int
		want          codes.Code
	}{
		{http.StatusBadRequest, codes.Internal},
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
		t.Run(fmt.Sprintf("backend_%d", tc.backendStatus), func(t *testing.T) {
			mock := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.backendStatus)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "error"})
			}))
			defer mock.Close()

			srv := newLokiErrServer(t, mock.URL)
			name := createGetLogRecord(t, srv, true)
			mockServer := &mockGetLogServer{ctx: context.Background()}

			err := srv.LogPluginServer.GetLog(&pb3.GetLogRequest{Name: name}, mockServer)
			if err == nil {
				t.Fatalf("backend HTTP %d: expected GetLog to return an error", tc.backendStatus)
			}
			if got := status.Code(err); got != tc.want {
				t.Fatalf("backend HTTP %d: got code %v, want %v (err: %v)", tc.backendStatus, got, tc.want, err)
			}
			if mockServer.receivedData != nil && mockServer.receivedData.Len() > 0 {
				t.Errorf("expected no log data to be streamed on failure, got %q", mockServer.receivedData.String())
			}
		})
	}
}

// TestGetLog_TransportErrorReturnsUnavailable covers the case where the backend
// cannot be reached at all.
func TestGetLog_TransportErrorReturnsUnavailable(t *testing.T) {
	mock := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	apiURL := mock.URL
	mock.Close() // connection now refused

	srv := newLokiErrServer(t, apiURL)
	name := createGetLogRecord(t, srv, true)
	mockServer := &mockGetLogServer{ctx: context.Background()}

	err := srv.LogPluginServer.GetLog(&pb3.GetLogRequest{Name: name}, mockServer)
	if err == nil {
		t.Fatal("expected GetLog to return a transport error")
	}
	if got := status.Code(err); got != codes.Unavailable {
		t.Fatalf("got %v, want Unavailable (err: %v)", got, err)
	}
}

// TestGetLog_NormalizesPlainErrorToInternal verifies that a backend error which
// carries no gRPC status is reported as Internal rather than the default
// Unknown that gRPC assigns to bare errors.
func TestGetLog_NormalizesPlainErrorToInternal(t *testing.T) {
	mock := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("backend must not be queried when the record has no start time")
		w.WriteHeader(http.StatusOK)
	}))
	defer mock.Close()

	srv := newLokiErrServer(t, mock.URL)
	name := createGetLogRecord(t, srv, false)
	mockServer := &mockGetLogServer{ctx: context.Background()}

	err := srv.LogPluginServer.GetLog(&pb3.GetLogRequest{Name: name}, mockServer)
	if err == nil {
		t.Fatal("expected GetLog to fail for a record without a start time")
	}
	if got := status.Code(err); got != codes.Internal {
		t.Fatalf("got %v, want Internal (err: %v)", got, err)
	}
}

// TestGetLog_SucceedsWithoutError guards against the error propagation change
// turning successful retrievals into failures.
func TestGetLog_SucceedsWithoutError(t *testing.T) {
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
	name := createGetLogRecord(t, srv, true)
	mockServer := &mockGetLogServer{ctx: context.Background()}

	if err := srv.LogPluginServer.GetLog(&pb3.GetLogRequest{Name: name}, mockServer); err != nil {
		t.Fatalf("GetLog returned unexpected error: %v", err)
	}
	if mockServer.receivedData == nil || mockServer.receivedData.String() != "hello world" {
		t.Fatalf("unexpected streamed data: %v", mockServer.receivedData)
	}
}
