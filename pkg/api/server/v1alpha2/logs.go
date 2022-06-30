package server

import (
	"bytes"
	"fmt"
	"io"

	"github.com/tektoncd/results/pkg/api/server/db"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	"github.com/tektoncd/results/pkg/logwriter"
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
	streamer, err := record.ToLogStreamer(dbRecord, s.logChunkSize, s.logDataDir)
	if err != nil {
		return err
	}
	// Step 5: Stream log via gRPC Send calls.
	_, err = streamer.WriteTo(logwriter.NewLogChunkWriter(srv, name, s.logChunkSize))
	return err
}

func (s *Server) PutLog(stream pb.Results_PutLogServer) error {
	var recordName string
	var bytesWritten int64
	var dbRecord *db.Record
	for {
		logChunk, err := stream.Recv()
		// Close the stream if we've reached the end
		if err == io.EOF {
			return stream.SendAndClose(&pb.PutLogSummary{
				Record:        recordName,
				BytesReceived: bytesWritten,
			})
		}
		// Ensure that we are receiving logs for the same record
		if recordName == "" {
			recordName = logChunk.GetName()
		}
		if recordName != logChunk.GetName() {
			return fmt.Errorf("cannot put logs for multiple records in the same stream")
		}

		// Step 1: Get the record by its name
		parent, result, name, err := record.ParseName(logChunk.GetName())
		if err != nil {
			return err
		}
		// Step 2: Run SAR check
		if err := s.auth.Check(stream.Context(), parent, auth.ResourceRecords, auth.PermissionUpdate); err != nil {
			return err
		}
		// Step 3: Get record from database
		if dbRecord == nil {
			dbRecord, err = getRecord(s.db.WithContext(stream.Context()), parent, result, name)
			if err != nil {
				return err
			}
		}
		// Step 4: Transform record into LogStreamer
		streamer, err := record.ToLogStreamer(dbRecord, s.logChunkSize, s.logDataDir)
		if err != nil {
			return err
		}
		// Step 5: Receive log data and store it
		buffer := bytes.NewBuffer(logChunk.GetData())
		written, err := streamer.ReadFrom(buffer)
		bytesWritten += written
		if err != nil {
			return err
		}
	}
}
