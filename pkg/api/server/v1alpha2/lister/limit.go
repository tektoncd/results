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
	"fmt"

	pagetokenpb "github.com/tektoncd/results/pkg/api/server/v1alpha2/lister/proto/pagetoken_go_proto"
	"gorm.io/gorm"
)

const (
	defaultPageSize = 50
	minPageSize     = 5
	maxPageSize     = 10000
)

type limit struct {
	pageSize int
}

// validateToken implements the queryBuilder interface.
func (l *limit) validateToken(token *pagetokenpb.PageToken) error {
	return nil
}

// build implements the queryBuilder interface.
func (l *limit) build(db *gorm.DB) (*gorm.DB, error) {
	if l.pageSize < minPageSize || l.pageSize > maxPageSize {
		return nil, fmt.Errorf("invalid page size (%d): value must be greater than %d and less than %d", l.pageSize, minPageSize, maxPageSize)
	}
	// Fetch n + 1 items to determine whether there are more pages and
	// therefore, whether a page token should be included in the response.
	db = db.Limit(l.pageSize + 1)
	return db, nil
}
