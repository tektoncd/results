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
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var allowedOrderByFields = []string{"created_time", "updated_time"}

// orderBy validates and returns a string formatted suitably for
// a sql order by clause.
func orderBy(fields string) (string, error) {
	if strings.TrimSpace(fields) == "" {
		return "", nil
	}

	var orderBy []string
	for _, field := range strings.Split(fields, ",") {
		ob, err := normalizeOrderByField(field)
		if err != nil {
			return "", err
		}
		orderBy = append(orderBy, ob)
	}

	return strings.Join(orderBy, ","), nil
}

// normalizeOrderByField takes a field string, validating and formatting
// it for use in a sql query. An error is returned if the format of the string
// doesn't match either "field_name" or "field_name direction".
func normalizeOrderByField(field string) (string, error) {
	f := strings.Fields(field)
	fieldName := ""
	direction := ""
	switch len(f) {
	case 1:
		fieldName = f[0]
	case 2:
		fieldName = f[0]
		direction = f[1]
	default:
		return "", status.Errorf(codes.InvalidArgument, "invalid order_by %q", field)
	}

	if !isAllowedField(fieldName) {
		return "", status.Errorf(codes.InvalidArgument, "order by %s not supported", fieldName)
	}

	if direction == "" {
		return fieldName, nil
	}
	return orderByDirection(fieldName, direction)
}

func isAllowedField(name string) bool {
	for i := range allowedOrderByFields {
		if name == allowedOrderByFields[i] {
			return true
		}
	}
	return false
}

func orderByDirection(field string, direction string) (string, error) {
	switch strings.ToLower(direction) {
	case "asc":
		return fmt.Sprintf("%s ASC", field), nil
	case "desc":
		return fmt.Sprintf("%s DESC", field), nil
	default:
		return "", status.Errorf(codes.InvalidArgument, "invalid sort direction %q", direction)
	}
}
