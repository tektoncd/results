package server

import (
	"context"

	"github.com/tektoncd/results/pkg/api/server/db/errors"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/lister"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/result"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	"github.com/tektoncd/results/pkg/api/server/db"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth"
	eventlist "github.com/tektoncd/results/pkg/api/server/v1alpha2/eventlist"
	"github.com/tektoncd/results/pkg/apis/v1alpha2"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

// GetEventList streams log record by log request
func (s *Server) GetEventList(ctx context.Context, req *pb.GetEventListRequest) (*pb.EventList, error) {
	parent, result, name, err := eventlist.ParseName(req.GetName())
	if err != nil {
		s.logger.Error(err)
		return nil, status.Error(codes.InvalidArgument, "Invalid Name")
	}

	if err := s.auth.Check(ctx, parent, auth.ResourceEventList, auth.PermissionGet); err != nil {
		return nil, err
	}

	rec, err := getRecord(s.db.WithContext(ctx), parent, result, name)
	if err != nil {
		return nil, err
	}

	// Check if the input record is referenced in any logs record in the result
	if rec.Type != v1alpha2.EventListRecordType {
		rec, err = getEventListRecord(s.db, parent, result, name)
		if err != nil {
			s.logger.Error(err)
			return nil, err
		}
	}

	out := &pb.EventList{
		Name: eventlist.FormatName(rec.Parent+"/results/"+rec.ResultName, rec.Name),
		Data: rec.Data,
	}
	return out, nil

}

func getEventListRecord(txn *gorm.DB, parent, result, name string) (*db.Record, error) {
	store := &db.Record{}
	q := txn.
		Where(&db.Record{Result: db.Result{Parent: parent, Name: result}}).
		Where("data -> 'spec' -> 'resource' ->> 'uid' =  ?", name).
		First(store)
	if err := errors.Wrap(q.Error); err != nil {
		return nil, err
	}
	return store, nil
}

// ListEventLists returns list of EventLists
func (s *Server) ListEventLists(ctx context.Context, req *pb.ListEventListsRequest) (*pb.ListEventListResponse, error) {
	if req.GetParent() == "" {
		return nil, status.Error(codes.InvalidArgument, "Parent missing")
	}
	parent, resultName, err := result.ParseName(req.GetParent())
	if err != nil {
		s.logger.Error(err)
		return nil, status.Error(codes.InvalidArgument, "Invalid Name")
	}
	if err := s.auth.Check(ctx, parent, auth.ResourceEventList, auth.PermissionList); err != nil {
		s.logger.Debug(err)
		// unauthenticated status code and debug message produced by Check
		return nil, err

	}

	eventListers, err := lister.OfEventLists(s.recordsEnv, parent, resultName, req)
	if err != nil {
		return nil, err
	}

	eventListsRecords, nextPageToken, err := eventListers.List(ctx, s.db)
	if err != nil {
		return nil, err
	}
	listEvents := make([]*pb.EventList, len(eventListsRecords))
	for i, rec := range eventListsRecords {
		parent, result, name, _ := eventlist.ParseName(rec.GetName())
		listEvents[i] = &pb.EventList{
			Name: eventlist.FormatName(parent+"/results/"+result, name),
			Data: rec.Data.Value,
		}
	}

	return &pb.ListEventListResponse{
		Eventlists:    listEvents,
		NextPageToken: nextPageToken,
	}, nil

}
