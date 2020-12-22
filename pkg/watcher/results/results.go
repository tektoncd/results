package results

import (
	"context"
	"fmt"
	"strings"

	"github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/result"
	"github.com/tektoncd/results/pkg/watcher/convert"
	"github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Client is a wrapper around a Results client that provides helpful utilities
// for performing result operations that require multiple RPCs or data specific
// operations.
type Client struct {
	pb.ResultsClient

	// We need to know what kind of type we're working with, since this
	// information is not returned back in Get requests.
	// See https://github.com/kubernetes/kubernetes/issues/3030 for more details.
	// We might be able to do something clever with schemes in the future.
	kind string
}

// NewClient returns a new results client for the particular kind.
func NewClient(client pb.ResultsClient, kind string) *Client {
	return &Client{
		ResultsClient: client,
		kind:          kind,
	}
}

// Put adds the given Object to the Results API.
// If the parent result is missing or the object is not yet associated with a
// result, one is created automatically.
// If the Object is already associated with a Record, the existing Record is
// updated - otherwise a new Record is created.
func (c *Client) Put(ctx context.Context, o metav1.Object, opts ...grpc.CallOption) (*pb.Result, *pb.Record, error) {
	// Make sure parent Result exists (or create one)
	result, err := c.ensureResult(ctx, o, opts...)
	if err != nil {
		return nil, nil, err
	}

	// Create or update the record.
	record, err := c.upsertRecord(ctx, result.GetName(), o, opts...)
	if err != nil {
		return nil, nil, err
	}

	return result, record, nil
}

// ensureResult gets the Result corresponding to the Object, or creates a new
// one.
func (c *Client) ensureResult(ctx context.Context, o metav1.Object, opts ...grpc.CallOption) (*pb.Result, error) {
	name, ok := o.GetAnnotations()[annotation.Result]
	if !ok {
		name = result.FormatName(o.GetNamespace(), c.defaultName(o))
	}
	res, err := c.ResultsClient.GetResult(ctx, &pb.GetResultRequest{Name: name}, opts...)
	if err != nil && status.Code(err) != codes.NotFound {
		return nil, status.Errorf(status.Code(err), "GetResult(%s): %v", name, err)
	}
	if err == nil {
		return res, nil
	}

	// Result doesn't exist yet - create.
	req := &pb.CreateResultRequest{
		Parent: o.GetNamespace(),
		Result: &pb.Result{
			Name: name,
		},
	}
	return c.ResultsClient.CreateResult(ctx, req, opts...)
}

// upsertRecord updates or creates a record for the object
func (c *Client) upsertRecord(ctx context.Context, parent string, o metav1.Object, opts ...grpc.CallOption) (*pb.Record, error) {
	name, ok := o.GetAnnotations()[annotation.Record]
	if !ok {
		name = record.FormatName(parent, c.defaultName(o))
	}

	data, err := convert.ToProto(o)
	if err != nil {
		return nil, err
	}

	curr, err := c.GetRecord(ctx, &pb.GetRecordRequest{Name: name}, opts...)
	if err != nil && status.Code(err) != codes.NotFound {
		return nil, err
	}
	if curr != nil {
		// Data already exists for the Record - update it.
		curr.Data = data
		return c.UpdateRecord(ctx, &pb.UpdateRecordRequest{
			Record: curr,
			Etag:   curr.GetEtag(),
		}, opts...)
	}

	// Data does not exist for the Record - create it.
	return c.CreateRecord(ctx, &pb.CreateRecordRequest{
		Parent: parent,
		Record: &pb.Record{
			Name: name,
			Data: data,
		},
	}, opts...)
}

// defaultName is the default Result/Record name that should be used if one is
// not already associated to the Object.
func (c *Client) defaultName(o metav1.Object) string {
	return strings.ToLower(fmt.Sprintf("%s-%s", c.kind, o.GetName()))
}
