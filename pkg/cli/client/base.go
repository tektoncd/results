package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/tektoncd/results/pkg/cli/common"

	"k8s.io/client-go/transport"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Error represents an error that occurred during a client operation
type Error struct {
	Message string
	Code    int
}

func (e *Error) Error() string {
	return fmt.Sprintf("client error: %s (code: %d)", e.Message, e.Code)
}

// NewError creates a new Error
func NewError(message string, code int) error {
	return &Error{
		Message: message,
		Code:    code,
	}
}

// Config for the HTTP client
type Config struct {
	URL       *url.URL
	Timeout   time.Duration
	Transport *transport.Config
}

// RESTClient handles HTTP communication with the server
type RESTClient struct {
	baseURL    *url.URL
	httpClient *http.Client
}

// NewRESTClient creates a new REST client.
func NewRESTClient(c *Config) (*RESTClient, error) {
	if c == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if c.URL == nil {
		return nil, fmt.Errorf("config.URL cannot be nil")
	}
	if c.Transport == nil {
		return nil, fmt.Errorf("config.Transport cannot be nil")
	}

	rt, err := transport.New(c.Transport)
	if err != nil {
		return nil, err
	}

	return &RESTClient{
		baseURL: c.URL,
		httpClient: &http.Client{
			Transport: rt,
			Timeout:   c.Timeout,
		},
	}, nil
}

// DoRequest performs an HTTP request and handles the response
func (c *RESTClient) DoRequest(ctx context.Context, method, url string, in proto.Message) (*common.Response, error) {
	var body io.Reader
	if in != nil {
		data, err := protojson.Marshal(in)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %v", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, NewError(string(body), resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	return common.NewResponse(data), nil
}

// BuildURL constructs a URL with the given path and query parameters
func (c *RESTClient) BuildURL(p string, params url.Values) string {
	u := *c.baseURL
	u.Path = path.Join(u.Path, p)
	if params != nil {
		u.RawQuery = params.Encode()
	}
	return u.String()
}
