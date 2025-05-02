package logs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/tektoncd/results/pkg/cli/client"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

// Client is a client for interacting with logs.
type Client struct {
	*client.RESTClient
}

// NewClient creates a new logs client.
func NewClient(rc *client.RESTClient) *Client {
	return &Client{RESTClient: rc}
}

// GetLog gets a log by name.
func (c *Client) GetLog(ctx context.Context, req *pb.GetLogRequest) (io.Reader, error) {
	// Create a pipe to stream the logs
	pr, pw := io.Pipe()

	// Start a goroutine to handle the streaming response
	go func() {
		defer pw.Close()

		// Build the URL for the log request, replacing "records" with "logs" in the path
		url := c.BuildURL(fmt.Sprintf("parents/%s", strings.Replace(req.Name, "records", "logs", 1)), nil)

		// Make the request using the RESTClient's DoRequest method
		resp, err := c.DoRequest(ctx, http.MethodGet, url, nil)
		if err != nil {
			pw.CloseWithError(fmt.Errorf("failed to get log: %v", err))
			return
		}

		// Write the log data to the pipe
		if _, err := pw.Write(resp.Body()); err != nil {
			pw.CloseWithError(fmt.Errorf("failed to write log data: %v", err))
			return
		}
	}()

	return pr, nil
}
