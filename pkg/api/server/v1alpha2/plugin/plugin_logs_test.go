package plugin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tektoncd/results/pkg/api/server/config"
	"github.com/tektoncd/results/pkg/api/server/logger"
	"github.com/tektoncd/results/pkg/api/server/test"
	server "github.com/tektoncd/results/pkg/api/server/v1alpha2"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/log"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/record"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tektoncd/results/pkg/internal/jsonutil"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	pb3 "github.com/tektoncd/results/proto/v1alpha3/results_go_proto"
	"google.golang.org/genproto/googleapis/api/httpbody"
	"google.golang.org/grpc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type mockGetLogServer struct {
	grpc.ServerStream
	ctx          context.Context
	receivedData *bytes.Buffer
}

func (m *mockGetLogServer) Send(chunk *httpbody.HttpBody) error {
	if m.receivedData == nil {
		m.receivedData = &bytes.Buffer{}
	}
	_, err := m.receivedData.Write(chunk.GetData())
	return err
}

func (m *mockGetLogServer) Context() context.Context {
	return m.ctx
}

func TestLogPluginServer_GetLog(t *testing.T) {

	// Create a mock Loki server
	mockLoki := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Log the received request for debugging
		t.Logf("Received request: %s %s", r.Method, r.URL.String())
		t.Logf("Received headers: %v", r.Header)

		response := map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"result": []map[string]interface{}{
					{
						"stream": map[string]string{},
						"values": [][]string{
							{"1625081600000000001", "Log Message 1"},
						},
					},
					{
						"stream": map[string]string{},
						"values": [][]string{
							{"1625081600000000003", "Log Message 3"},
						},
					},
					{
						"stream": map[string]string{},
						"values": [][]string{
							{"1625081600000000000", "Log Message 0"},
						},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer mockLoki.Close()

	tokenDir := t.TempDir()
	tokenPath := filepath.Join(tokenDir, "token")
	err := os.WriteFile(tokenPath, []byte("dummytoken"), 0600)
	if err != nil {
		t.Fatalf("Failed to create token file: %v", err)
	}

	srv, err := server.New(&config.Config{
		LOGS_API:                                true,
		LOGS_TYPE:                               "Loki",
		DB_ENABLE_AUTO_MIGRATION:                true,
		LOGGING_PLUGIN_TOKEN_PATH:               tokenPath,
		LOGGING_PLUGIN_PROXY_PATH:               "/app",
		LOGGING_PLUGIN_API_URL:                  mockLoki.URL,
		LOGGING_PLUGIN_TLS_VERIFICATION_DISABLE: true,
		LOGGING_PLUGIN_STATIC_LABELS:            "namespace=\"foo\"",
		LOGGING_PLUGIN_NAMESPACE_KEY:            "namespace",
		LOGGING_PLUGIN_QUERY_LIMIT:              1500,
		LOGGING_PLUGIN_QUERY_PARAMS:             "direction=forward",
	}, logger.Get("info"), test.NewDB(t))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ctx := context.Background()
	// Create a mock GetLogServer
	mockServer := &mockGetLogServer{
		ctx: ctx,
	}

	res, err := srv.CreateResult(ctx, &pb.CreateResultRequest{
		Parent: "foo",
		Result: &pb.Result{
			Name: "foo/results/bar",
		},
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
							StartTime: &metav1.Time{
								Time: time.Now(),
							},
							CompletionTime: &metav1.Time{
								Time: time.Now(),
							},
						},
					},
				}),
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	// Create a test request
	req := &pb3.GetLogRequest{
		Name: log.FormatName(res.GetName(), "baz"),
	}

	expectedData := "Log Message 0\nLog Message 1\nLog Message 3"
	// Call GetLog
	err = srv.LogPluginServer.GetLog(req, mockServer)
	if err != nil {
		t.Fatalf("GetLog returned unexpected error: %v", err)
	}

	// Assert expectations
	actualData := mockServer.receivedData.String()
	if expectedData != actualData {
		t.Errorf("expected to have received %q, got %q", expectedData, actualData)
	}

}
