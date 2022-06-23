package server

import (
	"bytes"
	"io"

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
	// Step 2: Run SAR check
	if err := s.auth.Check(srv.Context(), parent, auth.ResourceRecords, auth.PermissionGet); err != nil {
		return err
	}
	// Step 3: Get record from database
	dbRecord, err := getRecord(s.db.WithContext(srv.Context()), parent, result, name)
	if err != nil {
		return err
	}
	// Step 4: Transform record into LogStreamer
	streamer, err := record.ToLogStreamer(dbRecord)
	if err != nil {
		return err
	}
	// Step 5: Stream log via gRPC Send calls.
	_, err = streamer.WriteTo(NewLogWriter(srv))
	return err
}

func NewLogWriter(srv pb.Results_GetLogServer) io.Writer {
	return &bytes.Buffer{}
}
