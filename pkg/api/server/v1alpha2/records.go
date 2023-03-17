// Copyright 2020 The Tekton Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	"github.com/tektoncd/results/pkg/api/server/db"
	"github.com/tektoncd/results/pkg/api/server/db/errors"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/lister"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/result"
	"github.com/tektoncd/results/pkg/internal/protoutil"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

func (s *Server) CreateRecord(ctx context.Context, req *pb.CreateRecordRequest) (*pb.Record, error) {
	r := req.GetRecord()

	// Validate the incoming request
	parent, resultName, name, err := record.ParseName(r.GetName())
	if err != nil {
		return nil, err
	}
	if req.GetParent() != result.FormatName(parent, resultName) {
		return nil, status.Error(codes.InvalidArgument, "requested parent does not match resource name")
	}
	if err := s.auth.Check(ctx, parent, auth.ResourceRecords, auth.PermissionCreate); err != nil {
		return nil, err
	}

	// Look up the result ID from the name. This does not have to happen
	// transactionally with the insert since name<->ID mappings are immutable,
	// and if the parent result is deleted mid-request, the insert should
	// fail due to foreign key constraints.
	resultID, err := s.getResultID(ctx, parent, resultName)
	if err != nil {
		return nil, err
	}

	// Populate Result with server provided fields.
	protoutil.ClearOutputOnly(r)
	r.Id = uid()
	r.Uid = r.Id
	ts := timestamppb.New(clock.Now())
	r.CreatedTime = ts
	r.CreateTime = ts
	r.UpdatedTime = ts
	r.UpdateTime = ts

	store, err := record.ToStorage(parent, resultName, resultID, name, req.GetRecord(), s.config)
	if err != nil {
		return nil, err
	}
	if err := record.UpdateEtag(store); err != nil {
		return nil, err
	}
	q := s.db.WithContext(ctx).
		Model(store).
		Create(store).Error
	if err := errors.Wrap(q); err != nil {
		return nil, err
	}

	return record.ToAPI(store), nil
}

// resultID is a utility struct to extract partial Result data representing
// Result name <-> ID mappings.
type resultID struct {
	Name string
	ID   string
}

func (s *Server) getResultIDImpl(ctx context.Context, parent, result string) (string, error) {
	id := new(resultID)
	q := s.db.WithContext(ctx).
		Model(&db.Result{}).
		Where(&db.Result{Parent: parent, Name: result}).
		First(id)
	if err := errors.Wrap(q.Error); err != nil {
		return "", err
	}
	return id.ID, nil
}

// GetRecord returns a single Record.
func (s *Server) GetRecord(ctx context.Context, req *pb.GetRecordRequest) (*pb.Record, error) {
	parent, result, name, err := record.ParseName(req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.auth.Check(ctx, parent, auth.ResourceRecords, auth.PermissionGet); err != nil {
		return nil, err
	}

	r, err := getRecord(s.db.WithContext(ctx), parent, result, name)
	if err != nil {
		return nil, err
	}
	return record.ToAPI(r), nil
}

func getRecord(txn *gorm.DB, parent, result, name string) (*db.Record, error) {
	// Note: set the Parent, ResultName and Name fields in the model used to
	// query the database to take advantage of the records_by_name composite
	// index. Although the Name is an unique value as well, leveraging the
	// index speeds up the query significantly. See
	// https://github.com/tektoncd/results/issues/336.
	store := &db.Record{}
	q := txn.
		Where(&db.Record{Parent: parent, ResultName: result, Name: name}).
		First(store)
	if err := errors.Wrap(q.Error); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Server) ListRecords(ctx context.Context, req *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error) {
	if req.GetParent() == "" {
		return nil, status.Error(codes.InvalidArgument, "parent missing")
	}

	// Authentication
	parent, resultName, err := result.ParseName(req.GetParent())
	if err != nil {
		return nil, err
	}
	if err := s.auth.Check(ctx, parent, auth.ResourceRecords, auth.PermissionList); err != nil {
		return nil, err
	}

	recordsLister, err := lister.OfRecords(s.recordsEnv, parent, resultName, req)
	if err != nil {
		return nil, err
	}

	records, nextPageToken, err := recordsLister.List(ctx, s.db)
	if err != nil {
		return nil, err
	}

	// If we found no records, check if result exists so we can return NotFound
	if len(records) == 0 {
		_, err := getResultByParentName(s.db, parent, resultName)
		if err != nil {
			return nil, err
		}
	}

	return &pb.ListRecordsResponse{
		Records:       records,
		NextPageToken: nextPageToken,
	}, nil
}

// UpdateRecord updates a record in the database.
func (s *Server) UpdateRecord(ctx context.Context, req *pb.UpdateRecordRequest) (*pb.Record, error) {
	in := req.GetRecord()

	parent, result, name, err := record.ParseName(in.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.auth.Check(ctx, parent, auth.ResourceRecords, auth.PermissionUpdate); err != nil {
		return nil, err
	}

	protoutil.ClearOutputOnly(in)

	var out *pb.Record
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r, err := getRecord(tx, parent, result, name)
		if err != nil {
			return err
		}

		// If the user provided the Etag field, then make sure the value of this field matches what saved in the database.
		// See https://google.aip.dev/154 for more information.
		if req.GetEtag() != "" && req.GetEtag() != r.Etag {
			return status.Error(codes.FailedPrecondition, "the etag mismatches")
		}

		// Merge existing data with user request.
		pb := record.ToAPI(r)
		// TODO: field mask support.
		proto.Merge(pb, in)

		updateTime := timestamppb.New(clock.Now())
		pb.UpdatedTime = updateTime
		pb.UpdateTime = updateTime

		// Convert back to storage and store.
		s, err := record.ToStorage(r.Parent, r.ResultName, r.ResultID, r.Name, pb, s.config)
		if err != nil {
			return err
		}
		if err := record.UpdateEtag(s); err != nil {
			return err
		}
		if err := errors.Wrap(tx.Save(s).Error); err != nil {
			return err
		}

		pb.Etag = s.Etag
		out = pb
		return nil
	})
	return out, err
}

// DeleteRecord deletes a given record.
func (s *Server) DeleteRecord(ctx context.Context, req *pb.DeleteRecordRequest) (*empty.Empty, error) {
	parent, result, name, err := record.ParseName(req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.auth.Check(ctx, parent, auth.ResourceRecords, auth.PermissionDelete); err != nil {
		return &empty.Empty{}, err
	}

	// First get the current record. This ensures that we return NOT_FOUND if
	// the entry is already deleted.
	// This does not need to be done in the same transaction as to delete,
	// since the identifiers are immutable.
	r, err := getRecord(s.db, parent, result, name)
	if err != nil {
		return &empty.Empty{}, err
	}
	return &empty.Empty{}, errors.Wrap(s.db.WithContext(ctx).Delete(&db.Record{}, r).Error)
}

// recordCEL defines the CEL environment for querying Record data.
// Fields are broken up explicitly in order to support dynamic handling of the
// data field as a key-value document.
func recordCEL() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Types(&pb.Record{}),
		cel.Declarations(decls.NewVar("name", decls.String)),
		cel.Declarations(decls.NewVar("data_type", decls.String)),
		cel.Declarations(decls.NewVar("data", decls.Dyn)),
	)
}
