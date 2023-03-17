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
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	pagetokenpb "github.com/tektoncd/results/pkg/api/server/v1alpha2/lister/proto/pagetoken_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

const (
	defaultOrderByColumn    = "id"
	defaultOrderByDirection = "ASC"
)

var (
	resultFieldsToColumns = map[string]string{
		"create_time": "created_time",
		"update_time": "updated_time",

		// Fields of RecordSummary type.
		"summary.start_time": "recordsummary_start_time",
		"summary.end_time":   "recordsummary_end_time",
	}

	recordFieldsToColumns = map[string]string{
		"create_time": "created_time",
		"update_time": "updated_time",
	}

	orderByPattern = regexp.MustCompile(`^([\w\.]+)\s*(ASC|asc|DESC|desc)?$`)
)

type order struct {
	columnName string
	fieldName  string
	direction  string
}

// validateToken implements the queryBuilder interface.
func (o *order) validateToken(token *pagetokenpb.PageToken) error {
	// Validate the token only if the caller wants to sort the collection by
	// a custom field.
	if fieldName := o.fieldName; fieldName != "" {
		orderBy := token.LastItem.OrderBy
		if orderBy == nil {
			return errors.New("last_item.order_by: missing required field")
		}
		tokenDirection := pagetokenpb.Order_Direction_name[int32(orderBy.Direction)]
		if orderBy.FieldName != fieldName || tokenDirection != o.direction {
			return fmt.Errorf("the provided order by clause differs from the value passed in the previous query\nexpected: %s %s\ngot: %s %s",
				orderBy.FieldName, tokenDirection,
				fieldName, o.direction)
		}
	}
	return nil
}

// build implements the queryBuilder interface.
func (o *order) build(db *gorm.DB) (*gorm.DB, error) {
	direction := defaultOrderByDirection
	if o.direction != "" {
		direction = o.direction
	}
	if o.columnName != "" {
		db = db.Order(o.columnName + " " + direction)
	}
	return db.Order(defaultOrderByColumn + " " + direction), nil
}

// parseOrderBy attempts to parse the input into a suitable tuple of column and
// direction to be used in the sql order by clause.
func parseOrderBy(in string, fieldsToColumns map[string]string) (columnName string, fieldName string, direction string, err error) {
	in = strings.TrimSpace(in)
	if in == "" {
		return "", "", "", nil
	}

	matches := orderByPattern.FindStringSubmatch(in)
	if matches == nil {
		return "", "", "", status.Errorf(codes.InvalidArgument, "invalid order by statement:\n%s", explainOrderByFormat(fieldsToColumns))
	}

	fieldName = matches[1]
	columnName = fieldsToColumns[fieldName]
	if columnName == "" {
		return "", "", "", status.Errorf(codes.InvalidArgument, "%s: field is unknown or cannot be used in the order by clause\n%s", fieldName, explainOrderByFormat(fieldsToColumns))
	}

	if desiredDirection := matches[2]; desiredDirection == "" {
		direction = defaultOrderByDirection
	} else {
		direction = strings.ToUpper(strings.TrimSpace(matches[2]))
	}

	return
}

// explainOrderByFormat returns a descriptive message to inform callers on the
// OrderBy field's correct format.
func explainOrderByFormat(fieldsToColumns map[string]string) string {
	validFields := make([]string, 0, len(fieldsToColumns))
	for field := range fieldsToColumns {
		validFields = append(validFields, field)
	}
	sort.Strings(validFields)
	return fmt.Sprintf("the value must obey the format <%s> [asc|desc]", strings.Join(validFields, "|"))
}

// newOrder creates a new order object from the provided orderBy value.
func newOrder(orderBy string, fieldsToColumns map[string]string) (*order, error) {
	column, field, direction, err := parseOrderBy(orderBy, fieldsToColumns)
	if err != nil {
		return nil, err
	}
	return &order{
		columnName: column,
		fieldName:  field,
		direction:  direction,
	}, nil
}
