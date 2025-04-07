package records

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/tektoncd/results/pkg/cli/client"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

// RecordClient defines the interface for record-related operations
type RecordClient interface {
	ListRecords(ctx context.Context, in *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error)
}

// recordClient implements the RecordClient interface
type recordClient struct {
	*client.RESTClient
}

// NewClient creates a new record client
func NewClient(rc *client.RESTClient) RecordClient {
	return &recordClient{RESTClient: rc}
}

// ListRecords makes request to get record list
func (c *recordClient) ListRecords(ctx context.Context, in *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error) {
	out := &pb.ListRecordsResponse{}

	// Add query parameters
	params := url.Values{}
	if in.Filter != "" {
		params.Set("filter", in.Filter)
	}
	if in.OrderBy != "" {
		params.Set("order_by", in.OrderBy)
	}
	if in.PageSize > 0 {
		params.Set("page_size", fmt.Sprintf("%d", in.PageSize))
	}
	if in.PageToken != "" {
		params.Set("page_token", in.PageToken)
	}

	// Add fields parameter for partial response
	params.Set("fields", "records.name,records.uid,records.create_time,records.update_time,records.data.value.metadata,records.data.value.status,next_page_token")

	// Construct the URL with parents prefix
	buildURL := c.BuildURL(fmt.Sprintf("parents/%s/records", in.Parent), params)

	// Make the request
	return out, c.Send(ctx, http.MethodGet, buildURL, in, out)
}
