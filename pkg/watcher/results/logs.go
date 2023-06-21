package results

import (
	"context"

	"github.com/google/uuid"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/log"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	"github.com/tektoncd/results/pkg/watcher/convert"
	"github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// PutLog adds the given Object to the Results API.
// If the parent result is missing or the object is not yet associated with a
// result, one is created automatically.
func (c *Client) PutLog(ctx context.Context, o Object, opts ...grpc.CallOption) (*pb.Record, error) {
	res, err := c.ensureResult(ctx, o, opts...)
	if err != nil {
		return nil, err
	}
	return c.createLogRecord(ctx, res, o, opts...)
}

// createLogRecord creates a record for logs.
func (c *Client) createLogRecord(ctx context.Context, result *pb.Result, o Object, opts ...grpc.CallOption) (*pb.Record, error) {
	name, err := getLogRecordName(result, o)
	if err != nil {
		return nil, err
	}
	kind := o.GetObjectKind().GroupVersionKind().Kind
	rec, err := c.GetRecord(ctx, &pb.GetRecordRequest{Name: name}, opts...)
	if err != nil && status.Code(err) != codes.NotFound {
		return nil, err
	}
	if rec != nil {
		return rec, nil
	}
	data, err := convert.ToLogProto(o, kind, name)
	if err != nil {
		return nil, err
	}
	return c.CreateRecord(ctx, &pb.CreateRecordRequest{
		Parent: result.GetName(),
		Record: &pb.Record{
			Name: name,
			Data: data,
		},
	})
}

// getLogRecordName gets the log name to use for the given object.
// The name is derived from a known Tekton annotation if available, else
// the object's UID is used to create MD5 UUID.
func getLogRecordName(result *pb.Result, o Object) (string, error) {
	name, ok := o.GetAnnotations()[annotation.Log]
	if ok {
		_, _, name, err := log.ParseName(name)
		if err == nil {
			return record.FormatName(result.GetName(), name), nil
		}
	}
	uid, err := uuid.Parse(result.GetUid())
	if err != nil {
		return "", nil
	}
	return record.FormatName(result.GetName(), uuid.NewMD5(uid, []byte(o.GetUID())).String()), nil
}

// GetLogRecord returns log record using gRPC clients.
func (c *Client) GetLogRecord(ctx context.Context, o Object) (*pb.Record, error) {
	res, err := c.ensureResult(ctx, o)
	if err != nil {
		return nil, err
	}
	name, err := getLogRecordName(res, o)
	if err != nil {
		return nil, err
	}
	rec, err := c.GetRecord(ctx, &pb.GetRecordRequest{Name: name})
	if err != nil && status.Code(err) == codes.NotFound {
		return nil, nil
	}
	return rec, err
}
