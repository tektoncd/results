package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/cel-go/cel"
	celenv "github.com/tektoncd/results/pkg/api/server/cel"
	"github.com/tektoncd/results/pkg/api/server/db/errors"
	"github.com/tektoncd/results/pkg/api/server/db/pagination"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/result"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	"github.com/tektoncd/results/pkg/api/server/db"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/log"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	"github.com/tektoncd/results/pkg/apis/v1alpha2"
	"github.com/tektoncd/results/pkg/logs"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Server) GetLog(req *pb.GetLogRequest, srv pb.Logs_GetLogServer) error {
	parent, res, name, err := log.ParseName(req.GetName())
	if err != nil {
		s.logger.Error(err)
		return status.Error(codes.InvalidArgument, "Invalid Name")
	}

	if err := s.auth.Check(srv.Context(), parent, auth.ResourceLogs, auth.PermissionGet); err != nil {
		s.logger.Error(err)
		// unauthenticated status code and debug message produced by Check
		return err
	}

	rec, err := getRecord(s.db, parent, res, name)
	if err != nil {
		s.logger.Error(err)
		return err
	}
	// Check if the input record is referenced in any logs record in the result
	if rec.Type != v1alpha2.LogRecordType {
		rec, err = getLogRecord(s.db, parent, res, name)
		if err != nil {
			s.logger.Error(err)
			return err
		}
	}

	stream, object, err := log.ToStream(srv.Context(), rec, s.config)
	if err != nil {
		s.logger.Error(err)
		return status.Error(codes.Internal, "Error streaming log")
	}
	if object.Status.Size == 0 {
		s.logger.Errorf("no logs exist for %s", req.GetName())
		return status.Error(codes.NotFound, "Log doesn't exist")
	}

	writer := logs.NewBufferedWriter(srv, req.GetName(), s.config.LOGS_BUFFER_SIZE)
	if _, err = stream.WriteTo(writer); err != nil {
		s.logger.Error(err)
		return status.Error(codes.Internal, "Error streaming log")
	}
	_, err = writer.Flush()
	if err != nil {
		s.logger.Error(err)
		return status.Error(codes.Internal, "Error streaming log")
	}
	return nil
}

func getLogRecord(txn *gorm.DB, parent, result, name string) (*db.Record, error) {
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

func (s *Server) UpdateLog(srv pb.Logs_UpdateLogServer) error {
	var name string
	var bytesWritten int64
	var rec *db.Record
	var object *v1alpha2.Log
	var stream log.Stream
	defer func() {
		if stream != nil {
			if err := stream.Flush(); err != nil {
				s.logger.Error(err)
			}
		}
	}()
	for {
		recv, err := srv.Recv()
		// If we reach the end of the srv, we receive an io.EOF error
		if err != nil {
			return s.handleReturn(srv, rec, object, bytesWritten, err)
		}
		// Ensure that we are receiving logs for the same record
		if name == "" {
			name = recv.GetName()
			s.logger.Debugf("receiving logs for %s", name)
		}
		if name != recv.GetName() {
			err := fmt.Errorf("cannot put logs for multiple records in the same server")
			return s.handleReturn(srv,
				rec,
				object,
				bytesWritten,
				err)
		}

		parent, resultName, recordName, err := log.ParseName(name)
		if err != nil {
			return s.handleReturn(srv, rec, object, bytesWritten, err)
		}

		if err := s.auth.Check(srv.Context(), parent, auth.ResourceLogs, auth.PermissionUpdate); err != nil {
			return s.handleReturn(srv, rec, object, bytesWritten, err)
		}

		if rec == nil {
			rec, err = getRecord(s.db.WithContext(srv.Context()), parent, resultName, recordName)
			if err != nil {
				return s.handleReturn(srv, rec, object, bytesWritten, err)
			}

		}

		if stream == nil {
			stream, object, err = log.ToStream(srv.Context(), rec, s.config)
			if err != nil {
				return s.handleReturn(srv, rec, object, bytesWritten, err)
			}
		}

		buffer := bytes.NewBuffer(recv.GetData())
		written, err := stream.ReadFrom(buffer)
		bytesWritten += written

		if err != nil {
			return s.handleReturn(srv, rec, object, bytesWritten, err)
		}
	}
}

func (s *Server) handleReturn(srv pb.Logs_UpdateLogServer, rec *db.Record, log *v1alpha2.Log, written int64, returnErr error) error {
	// When the srv reaches the end, srv.Recv() returns an io.EOF error
	// Therefore we should not return io.EOF if it is received in this function.
	// Otherwise, we should return the original error and not mask any subsequent errors handling cleanup/return.

	// If no database record or Log, return the original error
	if rec == nil || log == nil {
		return returnErr
	}
	apiRec := record.ToAPI(rec)
	apiRec.UpdateTime = timestamppb.Now()
	if written > 0 {
		log.Status.Size = written
	}
	data, err := json.Marshal(log)
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

	_, err = s.UpdateRecord(srv.Context(), &pb.UpdateRecordRequest{
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
		s.logger.Debugf("received %d bytes for %s", written, apiRec.GetName())
		return srv.SendAndClose(&pb.LogSummary{
			Record:        apiRec.Name,
			BytesReceived: written,
		})
	}
	return returnErr
}

func isNilOrEOF(err error) bool {
	return err == nil || err == io.EOF
}

func (s *Server) ListLogs(ctx context.Context, req *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error) {
	if req.GetParent() == "" {
		return nil, status.Error(codes.InvalidArgument, "Parent missing")
	}
	parent, _, err := result.ParseName(req.GetParent())
	if err != nil {
		s.logger.Error(err)
		return nil, status.Error(codes.InvalidArgument, "Invalid Name")
	}
	if err := s.auth.Check(ctx, parent, auth.ResourceLogs, auth.PermissionList); err != nil {
		s.logger.Debug(err)
		// unauthenticated status code and debug message produced by Check
		return nil, err

	}

	userPageSize, err := pageSize(int(req.GetPageSize()))
	if err != nil {
		return nil, err
	}

	start, err := pageStart(req.GetPageToken(), req.GetFilter())
	if err != nil {
		return nil, err
	}

	sortOrder, err := orderBy(req.GetOrderBy())
	if err != nil {
		return nil, err
	}

	env, err := recordCEL()
	if err != nil {
		return nil, err
	}
	prg, err := celenv.ParseFilter(env, req.GetFilter())
	if err != nil {
		return nil, err
	}
	// Fetch n+1 items to get the next token.
	rec, err := s.getFilteredPaginatedSortedLogRecords(ctx, req.GetParent(), start, userPageSize+1, prg, sortOrder)
	if err != nil {
		return nil, err
	}

	// If we returned the full n+1 items, use the last element as the next page
	// token.
	var nextToken string
	if len(rec) > userPageSize {
		next := rec[len(rec)-1]
		var err error
		nextToken, err = pagination.EncodeToken(next.GetUid(), req.GetFilter())
		if err != nil {
			return nil, err
		}
		rec = rec[:len(rec)-1]
	}

	return &pb.ListRecordsResponse{
		Records:       rec,
		NextPageToken: nextToken,
	}, nil
}

// getFilteredPaginatedSortedLogRecords returns the specified number of results that
// match the given CEL program.
func (s *Server) getFilteredPaginatedSortedLogRecords(ctx context.Context, parent, start string, pageSize int, prg cel.Program, sortOrder string) ([]*pb.Record, error) {
	parent, resultName, err := result.ParseName(parent)
	if err != nil {
		return nil, err
	}

	rec := make([]*pb.Record, 0, pageSize)
	batcher := pagination.NewBatcher(pageSize, minPageSize, maxPageSize)
	for len(rec) < pageSize {
		batchSize := batcher.Next()
		dbrecords := make([]*db.Record, 0, batchSize)
		q := s.db.WithContext(ctx).Where("type = ?", v1alpha2.LogRecordType)
		q = q.Where("id > ?", start)
		// Specifying `-` allows users to read Records across Results.
		// See https://google.aip.dev/159 for more details.
		if parent != "-" {
			q = q.Where("parent = ?", parent)
		}
		if resultName != "-" {
			q = q.Where("result_name = ?", resultName)
		}
		if sortOrder != "" {
			q = q.Order(sortOrder)
		}
		q = q.Limit(batchSize).Find(&dbrecords)
		if err := errors.Wrap(q.Error); err != nil {
			return nil, err
		}

		// Only return results that match the filter.
		for _, r := range dbrecords {
			api := record.ToAPI(r)
			ok, err := record.Match(api, prg)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}

			// Change resource name to log format
			parent, resultName, recordName, err := record.ParseName(api.Name)
			if err != nil {
				return nil, err
			}
			api.Name = log.FormatName(result.FormatName(parent, resultName), recordName)

			rec = append(rec, api)
			if len(rec) >= pageSize {
				return rec, nil
			}
		}

		// We fetched fewer results than requested - this means we've exhausted all items.
		if len(dbrecords) < batchSize {
			break
		}

		// Set params for next batch.
		start = dbrecords[len(dbrecords)-1].ID
		batcher.Update(len(dbrecords), batchSize)
	}
	return rec, nil
}

// DeleteLog deletes a given record and the stored log.
func (s *Server) DeleteLog(ctx context.Context, req *pb.DeleteLogRequest) (*empty.Empty, error) {
	parent, res, name, err := log.ParseName(req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.auth.Check(ctx, parent, auth.ResourceLogs, auth.PermissionDelete); err != nil {
		return &empty.Empty{}, err
	}

	// Check in the input record exists in the database
	rec, err := getRecord(s.db, parent, res, name)
	if err != nil {
		return &empty.Empty{}, err
	}
	// Check if the input record is referenced in any logs record
	if rec.Type != v1alpha2.LogRecordType {
		rec, err = getLogRecord(s.db, parent, res, name)
		if err != nil {
			return &empty.Empty{}, err
		}
	}

	streamer, _, err := log.ToStream(ctx, rec, s.config)
	err = streamer.Delete()
	if err != nil {
		return nil, err
	}

	return &empty.Empty{}, errors.Wrap(s.db.WithContext(ctx).Delete(&db.Record{}, rec).Error)
}
