// Copyright 2021 The Tekton Authors
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
	"fmt"
	"regexp"
	"strings"

	"github.com/google/cel-go/cel"
	tdb "github.com/tektoncd/results/pkg/api/server/db"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"gorm.io/gorm"
)

const (
	durationQuery          = "(data->'status'->>'completionTime')::TIMESTAMP WITH TIME ZONE - (data->'status'->>'startTime')::TIMESTAMP WITH TIME ZONE"
	statusQuery            = "(data->'status'->'conditions'->0->>'reason')"
	groupByTimeQuery       = "(data->'metadata'->>'creationTimestamp')::TIMESTAMP WITH TIME ZONE"
	groupByParentQuery     = "data->'metadata'->>'namespace'"
	groupByPipelineQuery   = "data->'metadata'->'labels'->>'tekton.dev/pipeline'"
	groupByRepositoryQuery = "data->'metadata'->'annotations'->>'pipelinesascode.tekton.dev/repository'"
	startTimeQuery         = "(data->'status'->>'startTime')::TIMESTAMP WITH TIME ZONE"
)

type summaryRequest interface {
	GetParent() string
	GetFilter() string
	GetGroupBy() string
	GetSummary() string
	GetOrderBy() string
}

// Aggregator contains the query builders for filters and aggregate functions for summary
type Aggregator struct {
	queryBuilders []queryBuilder
	aggregators   []aggregateFunc
}

func newAggregator(env *cel.Env, aggregateObjectRequest summaryRequest, clauses ...equalityClause) (*Aggregator, error) {
	filters := &filter{
		env:             env,
		expr:            strings.TrimSpace(aggregateObjectRequest.GetFilter()),
		equalityClauses: clauses,
	}

	// Summary is required
	summary := strings.Split(strings.TrimSpace(aggregateObjectRequest.GetSummary()), ",")
	if len(summary) == 1 && summary[0] == "" {
		// include 'total' by default
		summary = append(summary, "total")
	}

	aggregators, err := getAggregateFunc(summary)
	if err != nil {
		return nil, err
	}

	// Group by is optional
	group := strings.TrimSpace(aggregateObjectRequest.GetGroupBy())
	if group != "" {
		groupQuery, err := checkAndBuildGroupQuery(group)
		if err != nil {
			return nil, err
		}
		aggregators = append(aggregators, groupBy(groupQuery))
	}

	orderQuery := strings.TrimSpace(aggregateObjectRequest.GetOrderBy())
	// Order by is only allowed when group by is present
	if orderQuery != "" && group != "" {
		orderSelect, err := checkAndBuildOrderBy(orderQuery, summary)
		if err != nil {
			return nil, err
		}
		aggregators = append(aggregators, orderBy(orderSelect))
	}

	return &Aggregator{
		aggregators: aggregators,
		queryBuilders: []queryBuilder{
			filters,
		},
	}, nil
}

// Aggregate function runs the aggregation tasks and returns Summary
func (a *Aggregator) Aggregate(ctx context.Context, db *gorm.DB) (*pb.RecordListSummary, error) {
	var err error
	summary := make([]map[string]interface{}, 0)
	db = db.Model(&tdb.Record{})
	db, err = a.buildQuery(ctx, db)
	if err != nil {
		return nil, err
	}

	db = a.applyAggregateFunc(ctx, db)
	db.Scan(&summary)

	sm, err := toSummary(summary)
	if err != nil {
		return nil, err
	}
	return sm, nil
}

// buildQuery applies filters
func (a *Aggregator) buildQuery(ctx context.Context, db *gorm.DB) (*gorm.DB, error) {
	var err error
	db = db.WithContext(ctx)

	for _, builder := range a.queryBuilders {
		db, err = builder.build(db)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}
	return db, err
}

// ToSummary converts the array of summary map to Summary proto
func toSummary(summary []map[string]interface{}) (*pb.RecordListSummary, error) {
	var data []*structpb.Struct
	for _, s := range summary {
		m := make(map[string]*structpb.Value)
		for sk, sv := range s {
			pbValue, err := structpb.NewValue(sv)
			if err != nil {
				return nil, err
			}
			m[sk] = pbValue
		}
		data = append(data, &structpb.Struct{Fields: m})
	}

	return &pb.RecordListSummary{
		Summary: data,
	}, nil
}

// aggregateFunc is a function that applies aggregate functions to the query
type aggregateFunc func(db *gorm.DB) *gorm.DB

var summaryFuncs = map[string]aggregateFunc{
	"total":          getCount("*", "total"),
	"avg_duration":   getDuration("AVG", durationQuery, "avg_duration"),
	"max_duration":   getDuration("MAX", durationQuery, "max_duration"),
	"total_duration": getDuration("SUM", durationQuery, "total_duration"),
	"min_duration":   getDuration("MIN", durationQuery, "min_duration"),
	"last_runtime":   getTime("MAX", startTimeQuery, "last_runtime"),
	"succeeded":      getStatus(statusQuery, "Succeeded"),
	"failed":         getStatus(statusQuery, "Failed"),
	"cancelled":      getStatus(statusQuery, "Cancelled"),
	"running":        getStatus(statusQuery, "Running"),
	"others":         getStatus(statusQuery, "Others"),
}

func getAggregateFunc(queries []string) ([]aggregateFunc, error) {
	fns := make([]aggregateFunc, 0, len(queries))
	for _, q := range queries {
		fn, ok := summaryFuncs[q]
		if !ok {
			return nil, status.Errorf(codes.InvalidArgument, "invalid aggregate query: %s", q)
		}
		fns = append(fns, fn)
	}
	return fns, nil
}

func (a *Aggregator) applyAggregateFunc(ctx context.Context, db *gorm.DB) *gorm.DB {
	db = db.WithContext(ctx)
	for _, fn := range a.aggregators {
		db = fn(db)
	}
	return db
}

func getStatus(query, reason string) aggregateFunc {
	return func(db *gorm.DB) *gorm.DB {
		statusSelect := ""
		switch reason {
		case "Succeeded":
			statusSelect = fmt.Sprintf("COUNT(CASE WHEN %s IN ('Succeeded', 'Completed') THEN 1 END) AS %s",
				query, strings.ToLower(reason))
		case "Others":
			statusSelect = fmt.Sprintf("COUNT(CASE WHEN %s NOT IN ('Failed', 'Succeeded', 'Cancelled', 'Running', 'Completed') THEN 1 END) AS %s",
				query, strings.ToLower(reason))
		default:
			statusSelect = fmt.Sprintf("COUNT(CASE WHEN %s = '%s' THEN 1 END) AS %s",
				query, reason, strings.ToLower(reason))
		}
		return db.Select(db.Statement.Selects, statusSelect)
	}
}

func getCount(query, countName string) aggregateFunc {
	return func(db *gorm.DB) *gorm.DB {
		return db.Select(db.Statement.Selects, fmt.Sprintf("COUNT(%s) AS %s", query, countName))
	}
}

func getDuration(fn, query, value string) aggregateFunc {
	return func(db *gorm.DB) *gorm.DB {
		return db.Select(db.Statement.Selects, fmt.Sprintf("%s(%s)::INTERVAL AS %s", fn, query, value))
	}
}

func getTime(fn, query, as string) aggregateFunc {
	return func(db *gorm.DB) *gorm.DB {
		return db.Select(db.Statement.Selects, fmt.Sprintf("%s(EXTRACT(EPOCH FROM %s)) AS %s", fn, query, as))
	}
}

var validGroups = map[string]bool{
	"year":       true,
	"month":      true,
	"week":       true,
	"day":        true,
	"hour":       true,
	"minute":     true,
	"pipeline":   false,
	"namespace":  false,
	"repository": false,
}

func groupBy(groupSelect string) aggregateFunc {
	return func(db *gorm.DB) *gorm.DB {
		return db.Select(db.Statement.Selects, groupSelect).Group("group_value")
	}
}

// checkAndBuildGroupQuery checks if the group by query is valid and returns the group by select query
func checkAndBuildGroupQuery(query string) (string, error) {
	parts := strings.Split(query, " ")
	isTime, ok := validGroups[parts[0]]
	if !ok {
		return "", status.Errorf(codes.InvalidArgument, "group_by does not recognize %s", query)
	}
	switch {
	case isTime && len(parts) == 1:
		return fmt.Sprintf("EXTRACT(EPOCH FROM DATE_TRUNC('%s', %s)) AS group_value", parts[0], groupByTimeQuery), nil
	case isTime && len(parts) == 2:
		if parts[1] != "completionTime" && parts[1] != "startTime" {
			return "", status.Errorf(codes.InvalidArgument, "group_by does not recognize %s", parts[1])
		}
		return fmt.Sprintf("EXTRACT(EPOCH FROM DATE_TRUNC('%s', (data->'status'->>'%s')::TIMESTAMP WITH TIME ZONE)) AS group_value", parts[0], parts[1]), nil
	case !isTime && len(parts) == 1:
		switch parts[0] {
		case "namespace":
			return fmt.Sprintf("%s AS group_value", groupByParentQuery), nil
		case "pipeline":
			// use 'namespace/pipeline' as group value because different namespaces may have pipelines with same name
			return fmt.Sprintf("CONCAT(%s, '/', %s) AS group_value", groupByParentQuery, groupByPipelineQuery), nil
		case "repository":
			return fmt.Sprintf("CONCAT(%s, '/', %s) AS group_value", groupByParentQuery, groupByRepositoryQuery), nil
		}
	default:
		return "", status.Errorf(codes.InvalidArgument, "group_by does not recognize %s", query)
	}
	return "", nil
}

func orderBy(orderSelect string) aggregateFunc {
	return func(db *gorm.DB) *gorm.DB {
		return db.Select(db.Statement.Selects).Order(orderSelect)
	}
}

// checkAndBuildOrderBy checks if the order by query is valid and returns the order by select query
func checkAndBuildOrderBy(query string, allowedFields []string) (string, error) {
	parts := strings.Split(query, " ")
	var orderByPattern = regexp.MustCompile(`^([\w\.]+)\s*(ASC|asc|DESC|desc)?$`)

	if len(parts) != 2 || !orderByPattern.MatchString(query) {
		return "", status.Errorf(codes.InvalidArgument, "order_by does not recognize %s", query)
	}

	fieldName := parts[0]
	orderDirection := strings.ToUpper(parts[1])

	// Check if the field name is in the list of allowed fields, must be one of the summary query
	isAllowedField := false
	for _, field := range allowedFields {
		if field == fieldName {
			isAllowedField = true
			break
		}
	}

	if !isAllowedField {
		return "", status.Errorf(codes.InvalidArgument, "field name %s is not allowed, must be one of summary", fieldName)
	}

	return fmt.Sprintf("%s %s", fieldName, orderDirection), nil
}

// OfRecordList returns a new Aggregator for Record List Summary Request
func OfRecordList(env *cel.Env, resultParent, resultName string, request *pb.RecordListSummaryRequest) (*Aggregator, error) {
	return newAggregator(env, request, equalityClause{
		columnName: "parent",
		value:      resultParent,
	}, equalityClause{
		columnName: "result_name",
		value:      resultName,
	})
}
