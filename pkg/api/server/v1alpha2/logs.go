package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"

	"github.com/tektoncd/results/pkg/api/server/db"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth"
	resultslog "github.com/tektoncd/results/pkg/api/server/v1alpha2/log"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	"github.com/tektoncd/results/pkg/apis/v1alpha2"
	logwriter "github.com/tektoncd/results/pkg/logwriter"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Server) GetLog(req *pb.GetLogRequest, srv pb.Results_GetLogServer) error {
	// Step 1: Get the record by its name
	parent, result, name, err := record.ParseName(req.GetName())
	if err != nil {
		fmt.Printf("GetLog: could not parse record name %s: %v\n", req.GetName(), err)
		return err
	}
	// Step 2: Run SAR check
	if err := s.auth.Check(srv.Context(), parent, auth.ResourceRecords, auth.PermissionGet); err != nil {
		fmt.Printf("GetLog: SAR check failed for %s: %v\n", req.GetName(), err)
		return err
	}
	// Step 3: Get record from database
	dbRecord, err := getRecord(s.db.WithContext(srv.Context()), parent, result, name)
	if err != nil {
		fmt.Printf("GetLog: could not get record for %s: %v\n", req.GetName(), err)
		return err
	}
	// Step 4: Transform record into LogStreamer
	streamer, taskRunLog, err := record.ToLogStreamer(dbRecord, s.Conf.LOG_CHUNK_SIZE, s.Conf.LOGS_DATA, s.Conf, s.ctx)
	if err != nil {
		fmt.Printf("GetLog: failed to convert %s to log streamer: %v\n", req.GetName(), err)
		return err
	}
	if !hasLogs(taskRunLog) {
		return fmt.Errorf("no logs exist for %s", req.GetName())
	}
	// Step 5: Stream log via gRPC Send calls.
	writer := logwriter.NewBufferedLogWriter(srv, name, s.Conf.LOG_CHUNK_SIZE)
	if _, err = streamer.WriteTo(writer); err != nil {
		return err
	}
	_, err = writer.WriteRemain()
	return err
}

func hasLogs(trl *v1alpha2.TaskRunLog) bool {
	if trl.Spec.Type != v1alpha2.FileLogType && trl.Spec.Type != v1alpha2.S3LogType {
		return false
	}
	if trl.Status.File == nil && trl.Status.S3Log == nil {
		return false
	}
	if trl.Status.File != nil {
		return trl.Status.File.Size > 0
	}
	if trl.Status.S3Log != nil {
		return trl.Status.S3Log.Size > 0
	}
	return false
}

func (s *Server) UpdateLog(stream pb.Results_UpdateLogServer) error {
	var recordName string
	var bytesWritten int64
	var dbRecord *db.Record
	var taskRunLog *v1alpha2.TaskRunLog
	var logStreamer resultslog.LogStreamer
	fmt.Printf("PutLog: begin file stream, bytes written %d \n", bytesWritten)
	defer func() {
		fmt.Printf("PutLog: finished file stream. Stored %d bytes.\n", bytesWritten)

		if flushable, ok := logStreamer.(resultslog.Flushable); ok {
			if err := flushable.Flush(); err != nil {
				log.Printf("failed to send logs %v", err)
			}
		}
	}()
	for {
		logChunk, err := stream.Recv()
		// If we reach the end of the stream, we receive an io.EOF error
		if err != nil {
			return s.handleReturn(stream, dbRecord, taskRunLog, bytesWritten, err)
		}
		// Ensure that we are receiving logs for the same record
		if recordName == "" {
			recordName = logChunk.GetName()
			fmt.Printf("PutLog: receiving logs for %s\n", recordName)
		}
		if recordName != logChunk.GetName() {
			err := fmt.Errorf("cannot put logs for multiple records in the same stream")
			fmt.Printf("PutLog: error streaming %s: %v\n", recordName, err)
			return s.handleReturn(stream,
				dbRecord,
				taskRunLog,
				bytesWritten,
				err)
		}

		// Step 1: Get the record by its name
		parent, result, name, err := record.ParseName(recordName)
		if err != nil {
			fmt.Printf("PutLog: error parsing record name %s: %v\n", recordName, err)
			return s.handleReturn(stream, dbRecord, taskRunLog, bytesWritten, err)
		}
		// Step 2: Run SAR check
		if err := s.auth.Check(stream.Context(), parent, auth.ResourceRecords, auth.PermissionUpdate); err != nil {
			fmt.Printf("PutLog: SAR check for %s failed: %v\n", recordName, err)
			return s.handleReturn(stream, dbRecord, taskRunLog, bytesWritten, err)
		}
		// Step 3: Get record from database
		if dbRecord == nil {
			dbRecord, err = getRecord(s.db.WithContext(stream.Context()), parent, result, name)
			if err != nil {
				fmt.Printf("PutLog: failed to find record for %s: %v\n", recordName, err)
				return s.handleReturn(stream, dbRecord, taskRunLog, bytesWritten, err)
			}

		}
		// Step 4: Transform record into LogStreamer
		if logStreamer == nil {
			logStreamer, taskRunLog, err = record.ToLogStreamer(dbRecord, s.Conf.LOG_CHUNK_SIZE, s.Conf.LOGS_DATA, s.Conf, s.ctx)
			if err != nil {
				fmt.Printf("PutLog: failed to create LogStreamer for %s: %v\n", recordName, err)
				return s.handleReturn(stream, dbRecord, taskRunLog, bytesWritten, err)
			}
		}
		// Step 5: Receive log data and store it
		buffer := bytes.NewBuffer(logChunk.GetData())
		written, err := logStreamer.ReadFrom(buffer)
		bytesWritten += written

		if err != nil {
			fmt.Printf("PutLog: failed to read from buffer for %s: %v\n", recordName, err)
			return s.handleReturn(stream, dbRecord, taskRunLog, bytesWritten, err)
		}
	}
}

func (s *Server) handleReturn(stream pb.Results_UpdateLogServer, rec *db.Record, trl *v1alpha2.TaskRunLog, written int64, returnErr error) error {
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
		fmt.Printf("handleReturn: failed to convert record %s to API: %v\n", rec.Name, err)
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
		fmt.Printf("handleReturn: failed to marshal %s to JSON: %v\n", rec.Name, err)
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
		fmt.Printf("handleReturn: failed to update record %s: %v\n", apiRec.GetName(), err)
		return err
	}

	if returnErr == io.EOF {
		fmt.Printf("PutLog: received %d bytes for %s\n", written, apiRec.GetName())
		return stream.SendAndClose(&pb.LogSummary{
			Record:        apiRec.Name,
			BytesReceived: written,
		})
	}
	return returnErr
}

func isNilOrEOF(err error) bool {
	return err == nil || err == io.EOF
}
