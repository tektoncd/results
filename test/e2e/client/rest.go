package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	v1alpha2 "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"k8s.io/client-go/transport"
)

const (
	listResultsPath   = "/apis/results.tekton.dev/v1alpha2/parents/%s/results"
	getResultsPath    = "/apis/results.tekton.dev/v1alpha2/parents/%s"
	deleteResultsPath = "/apis/results.tekton.dev/v1alpha2/parents/%s"
	listRecordsPath   = "/apis/results.tekton.dev/v1alpha2/parents/%s/records"
	getRecordPath     = "/apis/results.tekton.dev/v1alpha2/parents/%s"
	deleteRecordPath  = "/apis/results.tekton.dev/v1alpha2/parents/%s"
)

// RESTClient represents rest API client to connect to Tekton results api server.
type RESTClient interface {
	GetResult(ctx context.Context, in *v1alpha2.GetResultRequest) (*v1alpha2.Result, error)
	ListResults(ctx context.Context, in *v1alpha2.ListResultsRequest) (*v1alpha2.ListResultsResponse, error)
	DeleteResult(ctx context.Context, in *v1alpha2.DeleteResultRequest) (*emptypb.Empty, error)
	GetRecord(ctx context.Context, in *v1alpha2.GetRecordRequest) (*v1alpha2.Record, error)
	ListRecords(ctx context.Context, in *v1alpha2.ListRecordsRequest) (*v1alpha2.ListRecordsResponse, error)
	DeleteRecord(ctx context.Context, in *v1alpha2.DeleteRecordRequest) (*emptypb.Empty, error)
}

type restClient struct {
	httpClient *http.Client
	url        *url.URL
}

// NewRESTClient creates a new REST client.
func NewRESTClient(serverAddress string, opts ...RestOption) (RESTClient, error) {
	u, err := url.Parse(serverAddress)
	if err != nil {
		return nil, err
	}
	rc := &restClient{
		httpClient: &http.Client{},
		url:        u,
	}
	for _, option := range opts {
		err := option(rc)
		if err != nil {
			return nil, err
		}
	}

	return rc, nil
}

// RestOption is customization of the HTTP Client.
type RestOption func(client *restClient) error

// WithConfig allows customization of the HTTP Client.
func WithConfig(config *transport.Config) RestOption {
	return func(c *restClient) error {
		tc, err := transport.New(config)
		if err != nil {
			return err
		}
		c.httpClient.Transport = tc
		return nil
	}
}

// WithTimeout adds universal timeout for all HTTP request.
func WithTimeout(duration time.Duration) RestOption {
	return func(c *restClient) error {
		c.httpClient.Timeout = duration * time.Second
		return nil
	}
}

// TODO: Get these methods from a generated client

// GetResult makes request to get result
func (c *restClient) GetResult(ctx context.Context, in *v1alpha2.GetResultRequest) (*v1alpha2.Result, error) {
	out := &v1alpha2.Result{}
	return out, c.send(ctx, http.MethodGet, fmt.Sprintf(getResultsPath, in.Name), in, out)
}

// ListResults makes request and get result list
func (c *restClient) ListResults(ctx context.Context, in *v1alpha2.ListResultsRequest) (*v1alpha2.ListResultsResponse, error) {
	out := &v1alpha2.ListResultsResponse{}
	return out, c.send(ctx, http.MethodGet, fmt.Sprintf(listResultsPath, in.Parent), in, out)
}

// DeleteResult makes request to delete result
func (c *restClient) DeleteResult(ctx context.Context, in *v1alpha2.DeleteResultRequest) (*emptypb.Empty, error) {
	out := &emptypb.Empty{}
	return &emptypb.Empty{}, c.send(ctx, http.MethodDelete, fmt.Sprintf(deleteResultsPath, in.Name), in, out)
}

// GetRecord makes request to get record
func (c *restClient) GetRecord(ctx context.Context, in *v1alpha2.GetRecordRequest) (*v1alpha2.Record, error) {
	out := &v1alpha2.Record{}
	return out, c.send(ctx, http.MethodGet, fmt.Sprintf(getRecordPath, in.Name), in, out)
}

// ListRecords makes request to get record list
func (c *restClient) ListRecords(ctx context.Context, in *v1alpha2.ListRecordsRequest) (*v1alpha2.ListRecordsResponse, error) {
	out := &v1alpha2.ListRecordsResponse{}
	return out, c.send(ctx, http.MethodGet, fmt.Sprintf(listRecordsPath, in.Parent), in, out)
}

// DeleteRecord makes request to delete record
func (c *restClient) DeleteRecord(ctx context.Context, in *v1alpha2.DeleteRecordRequest) (*emptypb.Empty, error) {
	out := &emptypb.Empty{}
	return &emptypb.Empty{}, c.send(ctx, http.MethodDelete, fmt.Sprintf(deleteRecordPath, in.Name), in, out)
}

func (c *restClient) send(ctx context.Context, method, path string, in, out proto.Message) error {
	u := c.url.JoinPath(path)

	b, err := protojson.Marshal(in)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(b))
	if err != nil {
		return err
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}

	if res.StatusCode != http.StatusOK {
		return errors.New(http.StatusText(res.StatusCode))
	}

	defer res.Body.Close()
	b, err = io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	return protojson.Unmarshal(b, out)
}
