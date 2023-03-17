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
	"testing"
	"time"

	resultspb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/results/pkg/api/server/cel"
	pagetokenpb "github.com/tektoncd/results/pkg/api/server/v1alpha2/lister/proto/pagetoken_go_proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/utils/tests"
)

func TestBuildQuery(t *testing.T) {
	env, err := cel.NewResultsEnv()
	if err != nil {
		t.Fatal(err)
	}

	db, _ := gorm.Open(tests.DummyDialector{})
	statement := &gorm.Statement{DB: db, Clauses: map[string]clause.Clause{}}
	db.Statement = statement

	now := time.Now()

	order := &order{
		columnName: "created_time",
		direction:  "DESC",
	}

	token := &pagetokenpb.PageToken{
		Filter: `summary.status == SUCCESS`,
		LastItem: &pagetokenpb.Item{
			Uid: "bar",
			OrderBy: &pagetokenpb.Order{
				FieldName: "create_time",
				Value:     timestamppb.New(now),
				Direction: pagetokenpb.Order_DESC,
			},
		},
	}

	lister := &Lister[any, *resultspb.Result]{
		queryBuilders: []queryBuilder{
			&offset{
				order:     order,
				pageToken: token,
			},
			&filter{
				env: env,
				equalityClauses: []equalityClause{{
					columnName: "parent",
					value:      "foo",
				},
				},
				expr: token.Filter,
			},
			order,
			&limit{pageSize: 15},
		},
		pageToken: token,
	}

	t.Run("complex query", func(t *testing.T) {
		testDB, err := lister.buildQuery(context.Background(), db)
		if err != nil {
			t.Fatal(err)
		}

		testDB.Statement.Build("WHERE", "ORDER BY", "LIMIT")

		want := "WHERE (created_time, id) < (?, ?) AND parent = ? AND recordsummary_status = 1 ORDER BY created_time DESC,id DESC LIMIT 16"
		if got := testDB.Statement.SQL.String(); want != got {
			t.Errorf("Want %q, but got %q", want, got)
		}

		wantVars := []any{now, "bar", "foo"}
		if diff := cmp.Diff(wantVars, testDB.Statement.Vars); diff != "" {
			t.Errorf("Mismatch in the statement's vars:\n%s", diff)
		}
	})

	t.Run("return an error if the provided page token is invalid", func(t *testing.T) {
		token.Filter = `parent == "bar"`
		_, err := lister.buildQuery(context.Background(), db)
		if err == nil {
			t.Fatal("Want error, but got nil")
		}
		if !strings.Contains(err.Error(), "invalid page token") {
			t.Fatalf("Unexpected error %v", err)
		}
	})
}
