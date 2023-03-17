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
	"log"

	"github.com/golang/protobuf/ptypes/empty"
	"gorm.io/gorm"

	"github.com/tektoncd/results/pkg/api/server/db"
	"github.com/tektoncd/results/pkg/api/server/db/errors"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/lister"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/result"
	"github.com/tektoncd/results/pkg/internal/protoutil"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CreateResult creates a new result in the database.
func (s *Server) CreateResult(ctx context.Context, req *pb.CreateResultRequest) (*pb.Result, error) {
	r := req.GetResult()

	// Validate the incoming request
	parent, _, err := result.ParseName(r.GetName())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if req.GetParent() != parent {
		return nil, status.Error(codes.InvalidArgument, "requested parent does not match resource name")
	}
	if err := s.auth.Check(ctx, parent, auth.ResourceResults, auth.PermissionCreate); err != nil {
		return nil, err
	}

	// Populate Result with server provided fields.
	protoutil.ClearOutputOnly(r)
	id := uid()
	r.Id = id
	r.Uid = id
	ts := timestamppb.New(clock.Now())
	r.CreatedTime = ts
	r.CreateTime = ts
	r.UpdatedTime = ts
	r.UpdateTime = ts

	store, err := result.ToStorage(r)
	if err != nil {
		return nil, err
	}

	if err := result.UpdateEtag(store); err != nil {
		return nil, err
	}

	if err := errors.Wrap(s.db.WithContext(ctx).Create(store).Error); err != nil {
		return nil, err
	}
	return result.ToAPI(store), nil
}

// GetResult returns a single Result.
func (s *Server) GetResult(ctx context.Context, req *pb.GetResultRequest) (*pb.Result, error) {
	parent, name, err := result.ParseName(req.GetName())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err := s.auth.Check(ctx, parent, auth.ResourceResults, auth.PermissionGet); err != nil {
		return nil, err
	}
	store, err := getResultByParentName(s.db, parent, name)
	if err != nil {
		return nil, err
	}
	return result.ToAPI(store), nil
}

// UpdateResult updates a Result in the database.
func (s *Server) UpdateResult(ctx context.Context, req *pb.UpdateResultRequest) (*pb.Result, error) {
	// Retrieve result from database by name
	parent, name, err := result.ParseName(req.GetName())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err := s.auth.Check(ctx, parent, auth.ResourceResults, auth.PermissionUpdate); err != nil {
		return nil, err
	}

	var out *pb.Result
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		prev, err := getResultByParentName(tx, parent, name)
		if err != nil {
			return status.Errorf(codes.NotFound, "failed to find a result: %v", err)
		}

		// If the user provided the Etag field, then make sure the value of this field matches what saved in the database.
		// See https://google.aip.dev/154 for more information.
		if req.GetEtag() != "" && req.GetEtag() != prev.Etag {
			return status.Error(codes.FailedPrecondition, "the etag mismatches")
		}

		newpb := result.ToAPI(prev)
		reqpb := req.GetResult()
		protoutil.ClearOutputOnly(reqpb)
		// Merge requested Result with previous Result to apply updates,
		// making sure to filter out any OUTPUT_ONLY fields, and only
		// updatable fields.
		// We can't use proto.Merge, since empty fields in the req should take
		// precedence, so set each updatable field here.
		newpb.Annotations = reqpb.GetAnnotations()
		newpb.Summary = reqpb.GetSummary()
		toDB, err := result.ToStorage(newpb)
		if err != nil {
			return err
		}

		// Set server-side provided fields
		toDB.UpdatedTime = clock.Now()
		if err := result.UpdateEtag(toDB); err != nil {
			return err
		}

		// Write result back to database.
		if err = errors.Wrap(tx.Save(toDB).Error); err != nil {
			log.Printf("failed to save result into database: %v", err)
			return err
		}
		out = result.ToAPI(toDB)

		return nil
	})
	return out, err
}

// DeleteResult deletes a given result.
func (s *Server) DeleteResult(ctx context.Context, req *pb.DeleteResultRequest) (*empty.Empty, error) {
	parent, name, err := result.ParseName(req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.auth.Check(ctx, parent, auth.ResourceResults, auth.PermissionDelete); err != nil {
		return nil, err
	}

	// First get the current result. This ensures that we return NOT_FOUND if
	// the entry is already deleted.
	// This does not need to be done in the same transaction as to delete,
	// since the identifiers are immutable.
	r := &db.Result{}
	get := s.db.WithContext(ctx).
		Where(&db.Result{Parent: parent, Name: name}).
		First(r)
	if err := errors.Wrap(get.Error); err != nil {
		return &empty.Empty{}, err
	}

	// Delete the result.
	delete := s.db.WithContext(ctx).Delete(&db.Result{}, r)
	return &empty.Empty{}, errors.Wrap(delete.Error)
}

func (s *Server) ListResults(ctx context.Context, req *pb.ListResultsRequest) (*pb.ListResultsResponse, error) {
	if req.GetParent() == "" {
		return nil, status.Error(codes.InvalidArgument, "parent missing")
	}

	if err := s.auth.Check(ctx, req.GetParent(), auth.ResourceResults, auth.PermissionList); err != nil {
		return nil, err
	}

	resultsLister, err := lister.OfResults(s.resultsEnv, req)
	if err != nil {
		return nil, err
	}

	results, nextPageToken, err := resultsLister.List(ctx, s.db)
	if err != nil {
		return nil, err
	}

	return &pb.ListResultsResponse{
		Results:       results,
		NextPageToken: nextPageToken,
	}, nil
}

func getResultByParentName(gdb *gorm.DB, parent, name string) (*db.Result, error) {
	r := &db.Result{}
	q := gdb.
		Where(&db.Result{Parent: parent, Name: name}).
		First(r)
	if err := errors.Wrap(q.Error); err != nil {
		return nil, err
	}
	return r, nil
}
