package results

import (
	"context"

	"github.com/google/uuid"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	"github.com/tektoncd/results/pkg/apis/v1alpha3"
	"github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// PutEventList adds the given Object to the Results API.
// If the parent result is missing or the object is not yet associated with a
// result, one is created automatically.
func (c *Client) PutEventList(ctx context.Context, o Object, eventList []byte, opts ...grpc.CallOption) (*pb.Record, error) {
	res, err := c.ensureResult(ctx, o, opts...)
	if err != nil {
		return nil, err
	}
	return c.createEventListRecord(ctx, res, o, eventList, opts...)
}

// createEventListRecord creates a record for eventlist.
func (c *Client) createEventListRecord(ctx context.Context, result *pb.Result, o Object, eventList []byte, opts ...grpc.CallOption) (*pb.Record, error) {
	name, err := getEventListRecordName(result, o)
	if err != nil {
		return nil, err
	}
	rec, err := c.GetRecord(ctx, &pb.GetRecordRequest{Name: name}, opts...)
	if err != nil && status.Code(err) != codes.NotFound {
		return nil, err
	}
	if rec != nil {
		return rec, nil
	}
	return c.CreateRecord(ctx, &pb.CreateRecordRequest{
		Parent: result.GetName(),
		Record: &pb.Record{
			Name: name,
			Data: &pb.Any{
				Type:  v1alpha3.EventListRecordType,
				Value: eventList,
			},
		},
	})
}

// getEventListRecordName gets the eventlist name to use for the given object.
// The name is derived from a known Tekton annotation if available, else
// the object's UID is used to create MD5 UUID.
func getEventListRecordName(result *pb.Result, o Object) (string, error) {
	name, ok := o.GetAnnotations()[annotation.EventList]
	if ok {
		return name, nil
	}
	uid, err := uuid.Parse(result.GetUid())
	if err != nil {
		return "", nil
	}
	return FormatEventListName(result.GetName(), uid, o), nil
}

// FormatEventListName generates record name for EventList given resultName,
// result UUID and object - taskrun/pipelinerun.
func FormatEventListName(resultName string, resultUID uuid.UUID, o Object) string {
	return record.FormatName(resultName,
		uuid.NewMD5(resultUID, []byte(o.GetUID()+"eventlist")).String())
}

// GetEventListRecord returns eventlist record using gRPC clients.
func (c *Client) GetEventListRecord(ctx context.Context, o Object) (*pb.Record, error) {
	res, err := c.ensureResult(ctx, o)
	if err != nil {
		return nil, err
	}
	name, err := getEventListRecordName(res, o)
	if err != nil {
		return nil, err
	}
	rec, err := c.GetRecord(ctx, &pb.GetRecordRequest{Name: name})
	if err != nil && status.Code(err) == codes.NotFound {
		return nil, nil
	}
	return rec, err
}
