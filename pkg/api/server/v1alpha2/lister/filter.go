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
	"strings"

	pagetokenpb "github.com/tektoncd/results/pkg/api/server/v1alpha2/lister/proto/pagetoken_go_proto"

	"github.com/google/cel-go/cel"
	"github.com/tektoncd/results/pkg/api/server/cel2sql"
	"gorm.io/gorm"
)

type filter struct {
	env             *cel.Env
	expr            string
	equalityClauses []equalityClause
}

type equalityClause struct {
	columnName string
	value      any
}

// validateToken implements the queryBuilder interface.
func (f *filter) validateToken(token *pagetokenpb.PageToken) error {
	if strings.TrimSpace(f.expr) != strings.TrimSpace(token.Filter) {
		return fmt.Errorf("the provided filter differs from the filter used in the previous query\nexpected: %s\ngot: %s", token.Filter, f.expr)
	}
	return nil
}

// build implements the queryBuilder interface.
func (f *filter) build(db *gorm.DB) (*gorm.DB, error) {
	for _, clause := range f.equalityClauses {
		// Specifying `-` allows users to read Results/Records
		// without passing the parent.
		// See https://google.aip.dev/159 for more details.
		if clause.value == "-" {
			continue
		}
		db = db.Where(clause.columnName+" = ?", clause.value)
	}

	if expr := strings.TrimSpace(f.expr); expr != "" {
		sql, err := cel2sql.Convert(f.env, expr)
		if err != nil {
			return nil, err
		}
		db = db.Where(sql)
	}
	return db, nil
}
