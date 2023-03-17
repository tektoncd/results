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
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm/utils/tests"

	pagetokenpb "github.com/tektoncd/results/pkg/api/server/v1alpha2/lister/proto/pagetoken_go_proto"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestOffsetValidateToken(t *testing.T) {
	offset := &offset{}

	t.Run("valid", func(t *testing.T) {
		token := &pagetokenpb.PageToken{
			LastItem: &pagetokenpb.Item{
				Uid: "foo",
			},
		}
		if err := offset.validateToken(token); err != nil {
			t.Errorf("Want a valid page token, but got %v", err)
		}
	})

	t.Run("missing LastItem field", func(t *testing.T) {
		token := &pagetokenpb.PageToken{}
		if err := offset.validateToken(token); err == nil {
			t.Error("Want error, but got nil")
		}
	})

	t.Run("missing LastItem.Uid field", func(t *testing.T) {
		token := &pagetokenpb.PageToken{
			LastItem: &pagetokenpb.Item{},
		}
		if err := offset.validateToken(token); err == nil {
			t.Error("Want error, but got nil")
		}
	})
}

func TestOffsetBuild(t *testing.T) {
	db, _ := gorm.Open(tests.DummyDialector{})
	statement := &gorm.Statement{DB: db, Clauses: map[string]clause.Clause{}}
	db.Statement = statement

	t.Run("no clauses", func(t *testing.T) {
		offset := &offset{}
		testDB, err := offset.build(db)
		if err != nil {
			t.Fatal(err)
		}

		if got := len(testDB.Statement.Clauses); got != 0 {
			t.Errorf("Want 0 clauses in the statement, but got %d", got)
		}
	})

	t.Run("use only the id to determine the page offset", func(t *testing.T) {
		offset := &offset{
			pageToken: &pagetokenpb.PageToken{
				LastItem: &pagetokenpb.Item{
					Uid: "foo",
				},
			},
		}

		testDB, err := offset.build(db)
		if err != nil {
			t.Fatal(err)
		}

		testDB.Statement.Build("WHERE")

		want := "WHERE id > ?"
		if got := testDB.Statement.SQL.String(); want != got {
			t.Errorf("Want %q, but got %q", want, got)
		}

		wantVars := []any{"foo"}
		if diff := cmp.Diff(wantVars, testDB.Statement.Vars); diff != "" {
			t.Errorf("Mismatch in the statement's vars (-want +got):\n%s", diff)
		}
	})

	t.Run("use more than one field to determine the page offset", func(t *testing.T) {
		offset := &offset{
			order: &order{
				columnName: "created_time",
			},
			pageToken: &pagetokenpb.PageToken{
				LastItem: &pagetokenpb.Item{
					Uid: "foo",
					OrderBy: &pagetokenpb.Order{
						FieldName: "create_time",
						Value:     timestamppb.New(time.Now()),
						Direction: pagetokenpb.Order_ASC,
					},
				},
			},
		}

		testDB, err := offset.build(db)
		if err != nil {
			t.Fatal(err)
		}

		testDB.Statement.Build("WHERE")

		want := "WHERE (created_time, id) > (?, ?)"
		if got := testDB.Statement.SQL.String(); want != got {
			t.Errorf("Want %q, but got %q", want, got)
		}

		wantVars := []any{offset.pageToken.LastItem.OrderBy.Value.AsTime(), "foo"}
		if diff := cmp.Diff(wantVars, testDB.Statement.Vars); diff != "" {
			t.Errorf("Mismatch in the statement's vars (-want +got):\n%s", diff)
		}
	})

	t.Run("paginating results using descending order", func(t *testing.T) {
		offset := &offset{
			order: &order{
				columnName: "created_time",
			},
			pageToken: &pagetokenpb.PageToken{
				LastItem: &pagetokenpb.Item{
					Uid: "foo",
					OrderBy: &pagetokenpb.Order{
						FieldName: "create_time",
						Value:     timestamppb.New(time.Now()),
						Direction: pagetokenpb.Order_DESC,
					},
				},
			},
		}

		testDB, err := offset.build(db)
		if err != nil {
			t.Fatal(err)
		}

		testDB.Statement.Build("WHERE")

		want := "WHERE (created_time, id) < (?, ?)"
		if got := testDB.Statement.SQL.String(); want != got {
			t.Errorf("Want %q, but got %q", want, got)
		}

		wantVars := []any{offset.pageToken.LastItem.OrderBy.Value.AsTime(), "foo"}
		if diff := cmp.Diff(wantVars, testDB.Statement.Vars); diff != "" {
			t.Errorf("Mismatch in the statement's vars (-want +got):\n%s", diff)
		}
	})
}
