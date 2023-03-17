// Copyright 2023 The Tekton Authors
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

package lister

import (
	"context"
	"strings"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/google/cel-go/cel"
	"github.com/tektoncd/results/pkg/api/server/db"
	"github.com/tektoncd/results/pkg/api/server/db/errors"
	pagetokenpb "github.com/tektoncd/results/pkg/api/server/v1alpha2/lister/proto/pagetoken_go_proto"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/result"
	resultspb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// wireObject represents commonalities of Results and Records for the purposes of
// this package.
type wireObject interface {
	GetUid() string
	GetCreateTime() *timestamppb.Timestamp
	GetUpdateTime() *timestamppb.Timestamp
}

// request represents commonalities of ListResultsRequest and ListRecordsRequest
// objects.
type request interface {
	GetParent() string
	GetFilter() string
	GetOrderBy() string
	GetPageSize() int32
	GetPageToken() string
}

type queryBuilder interface {
	validateToken(token *pagetokenpb.PageToken) error
	build(db *gorm.DB) (*gorm.DB, error)
}

// Converter is a generic function which converts a database model to its wire
// form.
type Converter[M any, W wireObject] func(M) W

// PageTokenGenerator takes a wire object and returns a page token for
// retrieving more resources from thee API.
type PageTokenGenerator func(wireObject) (string, error)

// Lister is a generic utility to list, filter, sort and paginate Results and
// Records in a uniform and consistent manner.
type Lister[M any, W wireObject] struct {
	queryBuilders    []queryBuilder
	pageSize         int
	pageToken        *pagetokenpb.PageToken
	convert          Converter[M, W]
	genNextPageToken PageTokenGenerator
}

func (l *Lister[M, W]) buildQuery(ctx context.Context, db *gorm.DB) (*gorm.DB, error) {
	var err error
	db = db.WithContext(ctx)
	for _, builder := range l.queryBuilders {
		// First, let queryBuilders validate the incoming token if
		// applicable to make sure that the query parameters match those
		// passed in the previous request or that the token in question
		// wasn't improperly modified by the caller.
		if l.pageToken != nil {
			if err := builder.validateToken(l.pageToken); err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "invalid page token: %v", err)
			}
		}

		// Add clauses for filtering, sorting and paginating resources.
		db, err = builder.build(db)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}
	return db, nil
}

// List lists resources applying filters, sorting elements and handling
// pagination. It returns resources in their wire form and a token to be used
// later for retrieving more pages if applicable.
func (l *Lister[M, W]) List(ctx context.Context, db *gorm.DB) ([]W, string, error) {
	var err error
	db, err = l.buildQuery(ctx, db)
	if err != nil {
		return nil, "", err
	}

	var models = make([]M, 0)
	db.Find(&models)

	if err := errors.Wrap(err); err != nil {
		return nil, "", err
	}

	wire := make([]W, 0, len(models))
	for _, model := range models {
		wire = append(wire, l.convert(model))
	}

	var nextPageToken string
	if len(wire) > l.pageSize {
		// Generate the page token using the last resource in thee
		// returned collection, so it will be used as the starting point
		// for next queries.
		wire = wire[:l.pageSize]
		nextPageToken, err = l.genNextPageToken(wire[len(wire)-1])
	}

	return wire, nextPageToken, err
}

// OfResults creates a Lister for Result objects.
func OfResults(env *cel.Env, request *resultspb.ListResultsRequest) (*Lister[*db.Result, *resultspb.Result], error) {
	return newLister(env, resultFieldsToColumns, request, result.ToAPI, equalityClause{
		columnName: "parent",
		value:      strings.TrimSpace(request.GetParent()),
	})
}

func newLister[M any, W wireObject](env *cel.Env, fieldsToColumns map[string]string, listObjectsRequest request, convert Converter[M, W], clauses ...equalityClause) (*Lister[M, W], error) {
	pageToken, err := decodePageToken(strings.TrimSpace(listObjectsRequest.GetPageToken()))
	if err != nil {
		return nil, err
	}

	parent := strings.TrimSpace(listObjectsRequest.GetParent())
	if pageToken != nil && pageToken.Parent != parent {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page token: provided parent (%s) differs from the parent used in the previous query (%s)", parent, pageToken.Parent)
	}

	order, err := newOrder(strings.TrimSpace(listObjectsRequest.GetOrderBy()), resultFieldsToColumns)
	if err != nil {
		return nil, err
	}

	filter := &filter{
		env:             env,
		expr:            strings.TrimSpace(listObjectsRequest.GetFilter()),
		equalityClauses: clauses,
	}

	pageSize := listObjectsRequest.GetPageSize()
	if pageSize == 0 {
		pageSize = defaultPageSize
	}

	return &Lister[M, W]{
		queryBuilders: []queryBuilder{
			&offset{order: order, pageToken: pageToken},
			filter,
			order,
			&limit{pageSize: int(pageSize)},
		},
		pageSize:         int(pageSize),
		pageToken:        pageToken,
		convert:          convert,
		genNextPageToken: makePageTokenGenerator(parent, filter.expr, order),
	}, nil
}

func makePageTokenGenerator(parent, filter string, order *order) PageTokenGenerator {
	return func(obj wireObject) (string, error) {
		pageToken := &pagetokenpb.PageToken{
			Parent: parent,
			Filter: filter,
			LastItem: &pagetokenpb.Item{
				Uid: obj.GetUid(),
			},
		}

		if fieldName := order.fieldName; fieldName != "" {
			pageToken.LastItem.OrderBy = &pagetokenpb.Order{
				FieldName: fieldName,
				Value:     getTimestamp(obj, fieldName),
				Direction: pagetokenpb.Order_Direction(pagetokenpb.Order_Direction_value[order.direction]),
			}
		}

		return encodePageToken(pageToken)
	}
}

func getTimestamp(in wireObject, fieldName string) (timestamp *timestamppb.Timestamp) {
	switch fieldName {
	case "create_time":
		timestamp = in.GetCreateTime()

	case "update_time":
		timestamp = in.GetUpdateTime()

	case "summary.start_time":
		if result, ok := in.(*resultspb.Result); ok {
			if summary := result.Summary; summary != nil {
				timestamp = summary.GetStartTime()
			}
		}

	case "summary.end_time":
		if result, ok := in.(*resultspb.Result); ok {
			if summary := result.Summary; summary != nil {
				timestamp = summary.GetEndTime()
			}
		}
	}
	return
}

// OfRecords creates a Lister for Record objects.
func OfRecords(env *cel.Env, resultParent, resultName string, request *resultspb.ListRecordsRequest) (*Lister[*db.Record, *resultspb.Record], error) {
	return newLister(env, recordFieldsToColumns, request, record.ToAPI, equalityClause{
		columnName: "parent",
		value:      resultParent,
	},
		equalityClause{
			columnName: "result_name",
			value:      resultName,
		})
}
