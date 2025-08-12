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
	"testing"

	"github.com/google/go-cmp/cmp"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/utils/tests"
)

func TestLimitBuild(t *testing.T) {
	db, _ := gorm.Open(tests.DummyDialector{})
	statement := &gorm.Statement{DB: db, Clauses: map[string]clause.Clause{}}
	db.Statement = statement

	t.Run("limit clause", func(t *testing.T) {
		limit := &limit{pageSize: 10}

		testDB, err := limit.build(db)
		if err != nil {
			t.Fatal(err)
		}

		testDB.Statement.Build("LIMIT")

		wantSQL := "LIMIT ?"
		if got := testDB.Statement.SQL.String(); wantSQL != got {
			t.Errorf("Want SQL %q, but got %q", wantSQL, got)
		}

		wantVars := []interface{}{11}
		if diff := cmp.Diff(wantVars, testDB.Statement.Vars); diff != "" {
			t.Errorf("Mismatch in the statement's vars: %s", diff)
		}
	})

	t.Run("invalid page size - negative value", func(t *testing.T) {
		limit := &limit{pageSize: -1}

		_, err := limit.build(db)
		if err == nil {
			t.Fatal("Want error, but got nil")
		}
	})

	t.Run("invalid page size - too large value", func(t *testing.T) {
		limit := &limit{pageSize: 20000}

		_, err := limit.build(db)
		if err == nil {
			t.Fatal("Want error, but got nil")
		}
	})
}
