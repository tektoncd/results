package server

import (
	"context"

	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/lister"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/result"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GetRecordListSummary returns the summary and aggregation for a given list of records
func (s *Server) GetRecordListSummary(ctx context.Context, req *pb.RecordListSummaryRequest) (*pb.RecordListSummary, error) {
	if req.GetParent() == "" {
		return nil, status.Error(codes.InvalidArgument, "parent missing")
	}

	parent, resultName, err := result.ParseName(req.GetParent())
	if err != nil {
		return nil, err
	}

	if err := s.auth.Check(ctx, parent, auth.ResourceRecords, auth.PermissionGet); err != nil {
		return nil, err
	}

	recordAggregator, err := lister.OfRecordList(s.recordsEnv, parent, resultName, req)
	if err != nil {
		return nil, err
	}

	agg, err := recordAggregator.Aggregate(ctx, s.db)
	if err != nil {
		return nil, err
	}

	return agg, nil
}
