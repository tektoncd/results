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

	"gorm.io/gorm/utils/tests"

	pagetokenpb "github.com/tektoncd/results/pkg/api/server/v1alpha2/lister/proto/pagetoken_go_proto"

	"github.com/tektoncd/results/pkg/api/server/cel"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestFilterValidateToken(t *testing.T) {
	filter := &filter{expr: `parent == "foo"`}
	token := &pagetokenpb.PageToken{Filter: filter.expr}

	t.Run("valid token", func(t *testing.T) {
		if err := filter.validateToken(token); err != nil {
			t.Error(err)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		token.Filter = `parent == "bar"`
		if err := filter.validateToken(token); err == nil {
			t.Error("Want error, but got nil")
		}
	})
}

func TestFilterBuild(t *testing.T) {
	env, err := cel.NewResultsEnv()
	if err != nil {
		t.Fatal(err)
	}

	db, _ := gorm.Open(tests.DummyDialector{})
	statement := &gorm.Statement{DB: db, Clauses: map[string]clause.Clause{}}
	db.Statement = statement

	t.Run("no where clause", func(t *testing.T) {
		filter := &filter{}
		testDB, err := filter.build(db)
		if err != nil {
			t.Fatal(err)
		}

		if got := len(testDB.Statement.Clauses); got != 0 {
			t.Errorf("Want 0 clauses in the statement, but got %d", got)
		}
	})

	t.Run("do not add WHERE clauses if the user sends - as the value", func(t *testing.T) {
		filter := &filter{
			equalityClauses: []equalityClause{{
				columnName: "parent",
				value:      "-",
			},
			},
		}
		testDB, err := filter.build(db)
		if err != nil {
			t.Fatal(err)
		}

		if got := len(testDB.Statement.Clauses); got != 0 {
			t.Errorf("Want 0 clauses in the statement, but got %d", got)
		}
	})

	t.Run("where clause with parent and id", func(t *testing.T) {
		filter := &filter{
			env: env,
			equalityClauses: []equalityClause{
				{columnName: "parent", value: "foo"},
				{columnName: "id", value: "bar"},
			},
		}

		testDB, err := filter.build(db)
		if err != nil {
			t.Fatal(err)
		}

		testDB.Statement.Build("WHERE")

		want := "WHERE parent = ? AND id = ?"
		if got := testDB.Statement.SQL.String(); want != got {
			t.Errorf("Want %q, but got %q", want, got)
		}
	})

	t.Run("where clause with cel2sql filters", func(t *testing.T) {
		filter := &filter{
			env:  env,
			expr: `summary.status == SUCCESS`,
		}

		testDB, err := filter.build(db)
		if err != nil {
			t.Fatal(err)
		}

		testDB.Statement.Build("WHERE")

		want := "WHERE recordsummary_status = 1"
		if got := testDB.Statement.SQL.String(); want != got {
			t.Errorf("Want %q, but got %q", want, got)
		}
	})

	t.Run("more complex filter", func(t *testing.T) {
		filter := &filter{
			env: env,
			equalityClauses: []equalityClause{
				{columnName: "parent", value: "foo"},
				{columnName: "id", value: "bar"},
			},
			expr: "summary.status != SUCCESS",
		}

		testDB, err := filter.build(db)
		if err != nil {
			t.Fatal(err)
		}

		testDB.Statement.Build("WHERE")

		want := "WHERE parent = ? AND id = ? AND recordsummary_status <> 1"
		if got := testDB.Statement.SQL.String(); want != got {
			t.Errorf("Want %q, but got %q", want, got)
		}
	})
}
