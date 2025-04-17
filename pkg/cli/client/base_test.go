package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/protobuf/proto"
	k8stransport "k8s.io/client-go/transport"
)

func TestNewRESTClient(t *testing.T) {
	validURL, _ := url.Parse("http://localhost:8080")

	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				URL:     validURL,
				Timeout: 30 * time.Second,
				Transport: &k8stransport.Config{
					WrapTransport: func(rt http.RoundTripper) http.RoundTripper {
						return rt
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "nil transport",
			config: &Config{
				URL:       validURL,
				Timeout:   30 * time.Second,
				Transport: nil,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRESTClient(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewRESTClient() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBuildURL(t *testing.T) {
	baseURL, _ := url.Parse("http://localhost:8080")
	config := &Config{
		URL:     baseURL,
		Timeout: 30 * time.Second,
		Transport: &k8stransport.Config{
			WrapTransport: func(rt http.RoundTripper) http.RoundTripper {
				return rt
			},
		},
	}
	client, err := NewRESTClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	tests := []struct {
		name     string
		path     string
		params   url.Values
		expected string
	}{
		{
			name:     "no params",
			path:     "test",
			params:   nil,
			expected: "http://localhost:8080/test",
		},
		{
			name: "with params",
			path: "test",
			params: url.Values{
				"key1": []string{"value1"},
				"key2": []string{"value2"},
			},
			expected: "http://localhost:8080/test?key1=value1&key2=value2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := client.BuildURL(tt.path, tt.params)
			if url != tt.expected {
				t.Errorf("BuildURL() = %v, want %v", url, tt.expected)
			}
		})
	}
}

func TestSend(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Return a valid ListRecordsResponse JSON
		w.Write([]byte(`{
			"records": [
				{
					"name": "test-record",
					"uid": "test-uid",
					"create_time": "2024-03-21T00:00:00Z",
					"update_time": "2024-03-21T00:00:00Z",
					"data": {
						"type": "type.googleapis.com/tekton.results.v1alpha2.PipelineRun",
						"value": "eyJtZXRhZGF0YSI6eyJuYW1lIjoidGVzdC1yZWNvcmQiLCJuYW1lc3BhY2UiOiJ0ZXN0LW5zIn19"
					}
				}
			],
			"next_page_token": "test-token"
		}`))
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	config := &Config{
		URL:     serverURL,
		Timeout: 30 * time.Second,
		Transport: &k8stransport.Config{
			WrapTransport: func(rt http.RoundTripper) http.RoundTripper {
				return rt
			},
		},
	}
	client, err := NewRESTClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	tests := []struct {
		name    string
		method  string
		path    string
		in      proto.Message
		out     proto.Message
		wantErr bool
	}{
		{
			name:    "successful request",
			method:  http.MethodGet,
			path:    "test",
			in:      nil,
			out:     &pb.ListRecordsResponse{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := client.BuildURL(tt.path, nil)
			err := client.Send(context.Background(), tt.method, url, tt.in, tt.out)
			if (err != nil) != tt.wantErr {
				t.Errorf("Send() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
