package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/tektoncd/results/pkg/api/server/db"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth"
	resultslog "github.com/tektoncd/results/pkg/api/server/v1alpha2/log"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	"github.com/tektoncd/results/pkg/apis/v1alpha2"
	"github.com/tektoncd/results/pkg/logwriter"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/protobuf/types/known/timestamppb"
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
	streamer, _, err := record.ToLogStreamer(dbRecord, s.logChunkSize, s.logDataDir)
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
	var taskRunLog *v1alpha2.TaskRunLog
	var logStreamer resultslog.LogStreamer
	for {
		logChunk, err := stream.Recv()
		// If we reach the end of the stream, we receive an io.EOF error
		if err != nil {
			return s.handleReturn(stream, dbRecord, taskRunLog, bytesWritten, err)
		}
		// Ensure that we are receiving logs for the same record
		if recordName == "" {
			recordName = logChunk.GetName()
		}
		if recordName != logChunk.GetName() {
			return s.handleReturn(stream,
				dbRecord,
				taskRunLog,
				bytesWritten,
				fmt.Errorf("cannot put logs for multiple records in the same stream"))
		}

		// Step 1: Get the record by its name
		parent, result, name, err := record.ParseName(logChunk.GetName())
		if err != nil {
			return s.handleReturn(stream, dbRecord, taskRunLog, bytesWritten, err)
		}
		// Step 2: Run SAR check
		if err := s.auth.Check(stream.Context(), parent, auth.ResourceRecords, auth.PermissionUpdate); err != nil {
			return s.handleReturn(stream, dbRecord, taskRunLog, bytesWritten, err)
		}
		// Step 3: Get record from database
		if dbRecord == nil {
			dbRecord, err = getRecord(s.db.WithContext(stream.Context()), parent, result, name)
			if err != nil {
				return s.handleReturn(stream, dbRecord, taskRunLog, bytesWritten, err)
			}

		}
		// Step 4: Transform record into LogStreamer
		if logStreamer == nil {
			logStreamer, taskRunLog, err = record.ToLogStreamer(dbRecord, s.logChunkSize, s.logDataDir)
			if err != nil {
				return s.handleReturn(stream, dbRecord, taskRunLog, bytesWritten, err)
			}
		}
		// Step 5: Receive log data and store it
		buffer := bytes.NewBuffer(logChunk.GetData())
		written, err := logStreamer.ReadFrom(buffer)
		bytesWritten += written
		if err != nil {
			return s.handleReturn(stream, dbRecord, taskRunLog, bytesWritten, err)
		}
	}
}

func (s *Server) handleReturn(stream pb.Results_PutLogServer, rec *db.Record, trl *v1alpha2.TaskRunLog, written int64, returnErr error) error {
	// When the stream reaches the end, stream.Recv() returns an io.EOF error
	// Therefore we should not return io.EOF if it is received in this function.
	// Otherwise, we should return the original error and not mask any subsequent errors handling cleanup/return.

	// If no database record or TaskRunLog, return the original error
	if rec == nil || trl == nil {
		return returnErr
	}
	apiRec, err := record.ToAPI(rec)
	if err != nil {
		if !isNilOrEOF(returnErr) {
			return returnErr
		}
		return err
	}
	apiRec.UpdateTime = timestamppb.Now()
	if trl.Status.File == nil {
		trl.Status.File = &v1alpha2.FileLogTypeStatus{}
	}
	if written > 0 {
		trl.Status.File.Size = written
	}
	data, err := json.Marshal(trl)
	if err != nil {
		if !isNilOrEOF(returnErr) {
			return returnErr
		}
		return err
	}
	apiRec.Data = &pb.Any{
		Type:  rec.Type,
		Value: data,
	}

	_, err = s.UpdateRecord(stream.Context(), &pb.UpdateRecordRequest{
		Record: apiRec,
		Etag:   rec.Etag,
	})

	if err != nil {
		if !isNilOrEOF(returnErr) {
			return returnErr
		}
		return err
	}

	if returnErr == io.EOF {
		return stream.SendAndClose(&pb.PutLogSummary{
			Record:        apiRec.Name,
			BytesReceived: written,
		})
	}
	return returnErr
}

func isNilOrEOF(err error) bool {
	return err == nil || err == io.EOF
}
