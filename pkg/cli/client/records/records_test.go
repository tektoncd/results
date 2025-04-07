package records

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/tektoncd/results/pkg/cli/client"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	k8stransport "k8s.io/client-go/transport"
)

type mockTransport struct {
	listRecordsFunc func(*http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.listRecordsFunc(req)
}

func TestListRecords(t *testing.T) {
	tests := []struct {
		name          string
		req           *pb.ListRecordsRequest
		mockResponse  []byte
		mockError     error
		expectedError bool
	}{
		{
			name: "successful list",
			req: &pb.ListRecordsRequest{
				Parent:   "test-ns/results/-",
				PageSize: 10,
			},
			mockResponse: []byte(`{
				"records": [
					{
						"name": "test-ns/results/-/records/test-record",
						"uid": "test-uid",
						"data": {
							"type": "tekton.dev/v1beta1.PipelineRun",
							"value": "eyJtZXRhZGF0YSI6eyJuYW1lIjoidGVzdC1yZWNvcmQiLCJuYW1lc3BhY2UiOiJ0ZXN0LW5zIn19"
						},
						"create_time": "2024-03-20T00:00:00Z",
						"update_time": "2024-03-20T00:00:00Z"
					}
				],
				"next_page_token": ""
			}`),
			expectedError: false,
		},
		{
			name: "error response",
			req: &pb.ListRecordsRequest{
				Parent:   "test-ns/results/-",
				PageSize: 10,
			},
			mockError:     client.NewError("test error", 500),
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseURL, _ := url.Parse("http://localhost:8080")
			transport := &mockTransport{
				listRecordsFunc: func(_ *http.Request) (*http.Response, error) {
					if tt.mockError != nil {
						return &http.Response{
							StatusCode: 500,
							Body:       http.NoBody,
						}, tt.mockError
					}
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader(tt.mockResponse)),
						Header:     make(http.Header),
					}, nil
				},
			}

			config := &client.Config{
				URL:     baseURL,
				Timeout: 30 * time.Second,
				Transport: &k8stransport.Config{
					WrapTransport: func(_ http.RoundTripper) http.RoundTripper {
						return transport
					},
				},
			}

			restClient, err := client.NewRESTClient(config)
			if err != nil {
				t.Fatalf("Failed to create REST client: %v", err)
			}

			recordClient := NewClient(restClient)
			resp, err := recordClient.ListRecords(context.Background(), tt.req, "")

			if (err != nil) != tt.expectedError {
				t.Errorf("ListRecords() error = %v, wantErr %v", err, tt.expectedError)
				return
			}

			if !tt.expectedError && resp == nil {
				t.Error("ListRecords() response is nil")
			}
		})
	}
}
