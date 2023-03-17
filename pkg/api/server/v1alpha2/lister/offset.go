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

	pagetokenpb "github.com/tektoncd/results/pkg/api/server/v1alpha2/lister/proto/pagetoken_go_proto"
	"gorm.io/gorm"
)

type offset struct {
	order     *order
	pageToken *pagetokenpb.PageToken
}

// validateToken implements the queryBuilder interface.
func (o *offset) validateToken(token *pagetokenpb.PageToken) error {
	lastItem := token.LastItem
	if lastItem == nil {
		return errors.New("last_item: missing required field")
	}
	if lastItem.Uid == "" {
		return errors.New("last_item.uid: missing required field")
	}
	return nil
}

// build implements the queryBuilder interface.
func (o *offset) build(db *gorm.DB) (*gorm.DB, error) {
	if o.pageToken != nil {
		if lastItem := o.pageToken.LastItem; lastItem != nil {
			var leftHandSideExpression, rightHandSideExpression string
			comparisonOperator := ">"
			values := []any{}
			if orderBy := lastItem.OrderBy; orderBy != nil {
				leftHandSideExpression = fmt.Sprintf("(%s, %s)", o.order.columnName, defaultOrderByColumn)
				rightHandSideExpression = "(?, ?)"
				values = append(values, orderBy.Value.AsTime())
				if orderBy.Direction == pagetokenpb.Order_DESC {
					comparisonOperator = "<"
				}
			} else {
				leftHandSideExpression = defaultOrderByColumn
				rightHandSideExpression = "?"
			}
			values = append(values, lastItem.Uid)
			db = db.Where(fmt.Sprintf("%s %s %s", leftHandSideExpression, comparisonOperator, rightHandSideExpression), values...)
		}
	}
	return db, nil
}
