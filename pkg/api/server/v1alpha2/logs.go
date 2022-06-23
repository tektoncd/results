package server

import (
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

func (s *Server) GetLog(req *pb.GetLogRequest, srv pb.Results_GetLogServer) error {
	// Step 1: Get the record by its name
	parent, result, name, err := record.ParseName(req.GetName())
	if err != nil {
		return err
	}
	if err := s.auth.Check(srv.Context(), parent, auth.ResourceRecords, auth.PermissionGet); err != nil {
		return err
	}
	_, err = getRecord(s.db.WithContext(srv.Context()), parent, result, name)
	if err != nil {
		return err
	}
	return nil
}
