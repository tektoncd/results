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

	"github.com/google/go-cmp/cmp"
	pagetokenpb "github.com/tektoncd/results/pkg/api/server/v1alpha2/lister/proto/pagetoken_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestOrderByValidateToken(t *testing.T) {
	t.Run("valid token", func(t *testing.T) {
		order := &order{}
		token := &pagetokenpb.PageToken{}

		if err := order.validateToken(token); err != nil {
			t.Error(err)
		}
	})

	t.Run("the provided ordering values match the ordering values in the token", func(t *testing.T) {
		order := &order{
			fieldName: "create_time",
			direction: defaultOrderByDirection,
		}
		token := &pagetokenpb.PageToken{
			LastItem: &pagetokenpb.Item{
				OrderBy: &pagetokenpb.Order{
					FieldName: "create_time",
					Direction: pagetokenpb.Order_ASC,
				},
			},
		}

		if err := order.validateToken(token); err != nil {
			t.Error(err)
		}
	})

	t.Run("missing LastItem.OrderBy field", func(t *testing.T) {
		order := &order{fieldName: "create_time"}
		token := &pagetokenpb.PageToken{LastItem: &pagetokenpb.Item{}}

		if err := order.validateToken(token); err == nil {
			t.Error("Want error, but got nil")
		}
	})

	t.Run("the provided field name differs from the field name in the token", func(t *testing.T) {
		order := &order{
			fieldName: "create_time",
			direction: defaultOrderByDirection,
		}
		token := &pagetokenpb.PageToken{
			LastItem: &pagetokenpb.Item{
				OrderBy: &pagetokenpb.Order{
					FieldName: "update_time",
					Direction: pagetokenpb.Order_ASC,
				},
			},
		}

		if err := order.validateToken(token); err == nil {
			t.Error("Want error, but got nil")
		}
	})

	t.Run("the provided direction differs from the direction in the token", func(t *testing.T) {
		order := &order{
			fieldName: "create_time",
			direction: "DESC",
		}
		token := &pagetokenpb.PageToken{
			LastItem: &pagetokenpb.Item{
				OrderBy: &pagetokenpb.Order{
					FieldName: "create_time",
					Direction: pagetokenpb.Order_ASC,
				},
			},
		}

		if err := order.validateToken(token); err == nil {
			t.Error("Want error, but got nil")
		}
	})
}

func TestOrderByBuild(t *testing.T) {
	db, _ := gorm.Open(tests.DummyDialector{})
	statement := &gorm.Statement{DB: db, Clauses: map[string]clause.Clause{}}
	db.Statement = statement

	t.Run("no order by clause", func(t *testing.T) {
		order := &order{}

		testDB, err := order.build(db)
		if err != nil {
			t.Fatal(err)
		}

		testDB.Statement.Build("ORDER BY")

		want := "ORDER BY id ASC"
		if got := testDB.Statement.SQL.String(); want != got {
			t.Errorf("Want %q, but got %q", want, got)
		}
	})

	t.Run("order by a given column", func(t *testing.T) {
		order := &order{
			columnName: "created_time",
			direction:  "DESC",
		}

		testDB, err := order.build(db)
		if err != nil {
			t.Fatal(err)
		}

		testDB.Statement.Build("ORDER BY")

		want := "ORDER BY created_time DESC,id DESC"
		if got := testDB.Statement.SQL.String(); want != got {
			t.Errorf("Want %q, but got %q", want, got)
		}
	})
}

func TestParseOrderBy(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		column    string
		field     string
		direction string
	}{{
		name:      "valid order by statement",
		in:        "create_time DESC",
		column:    "created_time",
		field:     "create_time",
		direction: "DESC",
	},
		{
			name:      "sort in ascending order",
			in:        "create_time ASC",
			column:    "created_time",
			field:     "create_time",
			direction: "ASC",
		},
		{
			name:      "update_time field omitting the direction",
			in:        "update_time",
			column:    "updated_time",
			field:     "update_time",
			direction: "ASC",
		},
		{
			name:      "summary.start_time field",
			in:        "summary.start_time asc",
			column:    "recordsummary_start_time",
			field:     "summary.start_time",
			direction: "ASC",
		},
		{
			name:      "summary.end_time field",
			in:        "summary.end_time desc",
			column:    "recordsummary_end_time",
			field:     "summary.end_time",
			direction: "DESC",
		},
		{
			name:      "trailing and leading spaces",
			in:        "  summary.start_time   asc ",
			column:    "recordsummary_start_time",
			field:     "summary.start_time",
			direction: "ASC",
		},
		{
			name:      "trailing and leading spaces with no direction",
			in:        "  summary.start_time   ",
			column:    "recordsummary_start_time",
			field:     "summary.start_time",
			direction: "ASC",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotColumn, gotField, gotDirection, err := parseOrderBy(test.in, resultFieldsToColumns)
			if err != nil {
				t.Fatal(err)
			}

			if test.column != gotColumn {
				t.Errorf("Want column %q, but got %q", test.column, gotColumn)
			}

			if test.field != gotField {
				t.Errorf("Want field %q, but got %q", test.field, gotField)
			}

			if test.direction != gotDirection {
				t.Errorf("Want direction %q, but got %q", test.direction, gotDirection)
			}
		})
	}
}

func TestParseOrderByErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		err  error
	}{{
		name: "disallowed field in the order by clause",
		in:   "id",
		err:  status.Error(codes.InvalidArgument, "id: field is unknown or cannot be used in the order by clause\nthe value must obey the format <create_time|summary.end_time|summary.start_time|update_time> [asc|desc]"),
	},
		{
			name: "invalid order by",
			in:   "this is invalid",
			err:  status.Error(codes.InvalidArgument, "invalid order by statement:\nthe value must obey the format <create_time|summary.end_time|summary.start_time|update_time> [asc|desc]"),
		},
		{
			name: "invalid direction",
			in:   "create_time ASCC",
			err:  status.Error(codes.InvalidArgument, "invalid order by statement:\nthe value must obey the format <create_time|summary.end_time|summary.start_time|update_time> [asc|desc]"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, _, err := parseOrderBy(test.in, resultFieldsToColumns)
			if err == nil {
				t.Fatal("want error, but got nil")
			}

			if gotCode := status.Code(test.err); gotCode != status.Code(test.err) {
				t.Fatalf("Want code %d, but got %d", status.Code(test.err), gotCode)
			}

			if diff := cmp.Diff(test.err.Error(), err.Error()); diff != "" {
				t.Errorf("Mismatch in the error message (-want +got):\n%s", diff)
			}
		})
	}
}
