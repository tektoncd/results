package plugin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/tektoncd/results/pkg/api/server/config"
	"github.com/tektoncd/results/pkg/api/server/logger"
	"github.com/tektoncd/results/pkg/api/server/test"
	server "github.com/tektoncd/results/pkg/api/server/v1alpha2"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/log"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/plugin"
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
							{"1625081600000000001", "container-step-foo: Log Message 1"},
						},
					},
					{
						"stream": map[string]string{},
						"values": [][]string{
							{"1625081600000000003", "container-step-foo: Log Message 3"},
						},
					},
					{
						"stream": map[string]string{},
						"values": [][]string{
							{"1625081600000000000", "container-step-foo: Log Message 0"},
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
		LOGGING_PLUGIN_CONTAINER_KEY:            "kubernetes.container_name",
		LOGGING_PLUGIN_QUERY_LIMIT:              1500,
		LOGGING_PLUGIN_QUERY_PARAMS:             "direction=forward",
		LOGGING_PLUGIN_MULTIPART_REGEX:          "",
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

	expectedData := "container-step-foo: Log Message 0\ncontainer-step-foo: Log Message 1\ncontainer-step-foo: Log Message 3"
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

func TestMergeLogParts(t *testing.T) {
	tests := []struct {
		name           string
		regex          string
		logParts       []string
		expectedMerged [][]string
	}{
		{
			name:  "Test with matching regexp",
			regex: `-\d{10}.log$`,
			logParts: []string{
				"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/prepare-1743090392.log",
				"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/place-scripts-1743090392.log",
				"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/container-step-foo-1743090392.log",
				"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/container-step-foo-1743090554.log",
				"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/container-step-foo-1743090738.log",
			},
			expectedMerged: [][]string{
				{"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/prepare-1743090392.log"},
				{"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/place-scripts-1743090392.log"},
				{
					"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/container-step-foo-1743090392.log",
					"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/container-step-foo-1743090554.log",
					"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/container-step-foo-1743090738.log",
				},
			},
		},
		{
			name:  "Test with empty regexp",
			regex: ``,
			logParts: []string{
				"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/prepare-1743090392.log",
				"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/place-scripts-1743090392.log",
				"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/container-step-foo-1743090392.log",
				"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/container-step-foo-1743090554.log",
				"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/container-step-foo-1743090738.log",
			},
			expectedMerged: [][]string{
				{"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/prepare-1743090392.log"},
				{"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/place-scripts-1743090392.log"},
				{"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/container-step-foo-1743090392.log"},
				{"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/container-step-foo-1743090554.log"},
				{"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/container-step-foo-1743090738.log"},
			},
		},
		{
			name:  "Test with not matching regexp",
			regex: `not-matching-regex`,
			logParts: []string{
				"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/prepare-1743090392.log",
				"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/place-scripts-1743090392.log",
				"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/container-step-foo-1743090392.log",
				"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/container-step-foo-1743090554.log",
				"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/container-step-foo-1743090738.log",
			},
			expectedMerged: [][]string{
				{"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/prepare-1743090392.log"},
				{"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/place-scripts-1743090392.log"},
				{"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/container-step-foo-1743090392.log"},
				{"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/container-step-foo-1743090554.log"},
				{"/logs/default/0c8ca3dc-92ea-40df-aa0d-dff9f5361ae8/0f66649d-b8fa-4bb6-a42f-169d96c70298/container-step-foo-1743090738.log"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := regexp.MustCompile(tt.regex)
			result := plugin.MergeLogParts(tt.logParts, re)

			for i, parts := range result {
				for j, expectedPart := range parts {
					if expectedPart != tt.expectedMerged[i][j] {
						t.Errorf("Expected merged log part %d to be %q, got %q", i, tt.expectedMerged[i][j], expectedPart)
					}
				}
			}
		})
	}

}

func TestSplunkLogs(t *testing.T) {

	mockSplunk := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Log the received request for debugging
		t.Logf("Received request: %s %s", r.Method, r.URL.String())
		t.Logf("Received headers: %v", r.Header)

		// Verify the request path and query parameters
		switch r.URL.Path {
		case "/services/search/v2/jobs":
			w.WriteHeader(http.StatusCreated)
			w.Write(json.RawMessage(`{"sid":"1234567"}`))
			return
		case "/services/search/v2/jobs/1234567":
			w.WriteHeader(http.StatusOK)
			w.Write(json.RawMessage(`{
    "entry": [
        {
            "content": {
                "dispatchState": "DONE"
            }
        }
    ]
}`))
			return
		case "/services/search/v2/jobs/1234567/results":
			w.WriteHeader(http.StatusOK)
			w.Write(json.RawMessage(`{
    "rows": [
        [
            "foo",
			"",
            "step-test"
        ],
        [
            "bar",
			"",
            "step-test"
        ],
        [
            "",
			"foo-bar",
            "step-test"
        ]
    ]
}`))
		}
	}))
	defer mockSplunk.Close()

	os.Setenv("SPLUNK_SEARCH_TOKEN",
		"eyJraWQiOiJzcGx1bmsuc2VjcmV0IiwiYWxnIjoiSFM1MTIiLCJ2ZXIiOiJ2MiIsInR0eXAiOiJzdGF0aWMifQ.eyJpc3MiOiJzY19hZG1pbiBmcm9tIGZlZG9yYSIsInN1YiI6InNjX2FkbWluIiwiYXVkIjoia3ViZXJuZXRlcyIsImlkcCI6IlNwbHVuayIsImp0aSI6IjI0MGM1MDY3NGJkNDgxYjU5ZWE5MTY5ZDJjN2MyZjM5NDVmZDFhOTM3MWU0Yzg0MTQ0N2NkYTYzYmQ4NmZjMGQiLCJpYXQiOjE3NDYzODA1NDgsImV4cCI6MTc3MjM4OTc1NSwibmJyIjoxNzQ2MzgwNTQ4fQ.WnMJE6Dd0Fmn5AipZtl_bpfwIpfGR6feW63Xs1890XPh1o1CrBTNbslTeIH1b9ewluOfrY7rxToAQMoCO3ZJQA")

	cfg := &config.Config{
		LOGS_API:                                true,
		LOGS_TYPE:                               "Splunk",
		DB_ENABLE_AUTO_MIGRATION:                true,
		LOGGING_PLUGIN_API_URL:                  mockSplunk.URL,
		LOGGING_PLUGIN_NAMESPACE_KEY:            "kubernetes.namespace_name",
		LOGGING_PLUGIN_CONTAINER_KEY:            "kubernetes.container_name",
		LOGGING_PLUGIN_QUERY_PARAMS:             "index=konflux",
		LOGGING_PLUGIN_TLS_VERIFICATION_DISABLE: true,
	}

	srv, err := server.New(cfg, logger.Get("info"), test.NewDB(t))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ctx := context.Background()
	// Create a mock GetLogServer
	mockServer := &mockGetLogServer{
		ctx: ctx,
	}

	res, err := srv.CreateResult(ctx, &pb.CreateResultRequest{
		Parent: "rh-acs-tenant",
		Result: &pb.Result{
			Name: "rh-acs-tenant/results/test-result",
		},
	})
	if err != nil {
		t.Fatalf("CreateResult: %v", err)
	}

	_, err = srv.CreateRecord(ctx, &pb.CreateRecordRequest{
		Parent: res.GetName(),
		Record: &pb.Record{
			Name: record.FormatName(res.GetName(), "25274ae9-d521-4a9c-b254-122c17f64941"),
			Data: &pb.Any{
				Type: "tekton.dev/v1.TaskRun",
				Value: jsonutil.AnyBytes(t, pipelinev1.TaskRun{
					ObjectMeta: metav1.ObjectMeta{
						UID: "25274ae9-d521-4a9c-b254-122c17f64941",
					},
					Status: pipelinev1.TaskRunStatus{
						TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
							StartTime: &metav1.Time{
								Time: time.Now().Add(-time.Hour),
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
		Name: log.FormatName(res.GetName(), "25274ae9-d521-4a9c-b254-122c17f64941"),
	}

	// Call GetLog
	err = srv.LogPluginServer.GetLog(req, mockServer)
	t.Logf("recv error: %v", err)
	if err != nil {
		t.Fatalf("GetLog returned unexpected error: %v", err)
	}

	expectedData := "step-test:-\nfoo\nbar\nfoo-bar"

	if mockServer.receivedData == nil {
		t.Fatalf("no data received from GetLog")
	}

	actualData := mockServer.receivedData.String()
	if expectedData != actualData {
		t.Errorf("expected to have received %q, got %q", expectedData, actualData)
	}

}
