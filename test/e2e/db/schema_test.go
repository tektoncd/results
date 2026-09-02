//go:build e2e
// +build e2e

// Copyright 2026 The Tekton Authors
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

package db_test

import (
	"testing"

	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

// TestSchema_TablesExist verifies that the deployed API server's AutoMigrate
// created both core tables in the live Postgres instance.
func TestSchema_TablesExist(t *testing.T) {
	for _, table := range []string{"results", "records"} {
		t.Run(table, func(t *testing.T) {
			var exists bool
			err := rawDB.Raw(
				`SELECT EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = 'public' AND table_name = ?
				)`, table,
			).Scan(&exists).Error

			if err != nil {
				t.Fatalf("query failed: %v", err)
			}
			if !exists {
				t.Errorf("expected table %q to exist in public schema", table)
			}
		})
	}
}

// TestSchema_ColumnTypes verifies that GORM tags produce the correct Postgres
// column types on the live database. SQLite silently ignores type:jsonb
// and stores it as TEXT; Postgres must have actual jsonb.
func TestSchema_ColumnTypes(t *testing.T) {
	type columnSpec struct {
		table    string
		column   string
		wantType string
	}

	specs := []columnSpec{
		{table: "results", column: "parent", wantType: "character varying"},
		{table: "results", column: "id", wantType: "character varying"},
		{table: "results", column: "name", wantType: "character varying"},
		{table: "results", column: "annotations", wantType: "jsonb"},
		{table: "results", column: "etag", wantType: "character varying"},
		{table: "results", column: "created_time", wantType: "timestamp with time zone"},
		{table: "results", column: "updated_time", wantType: "timestamp with time zone"},

		{table: "records", column: "parent", wantType: "character varying"},
		{table: "records", column: "result_id", wantType: "character varying"},
		{table: "records", column: "id", wantType: "character varying"},
		{table: "records", column: "name", wantType: "character varying"},
		{table: "records", column: "data", wantType: "jsonb"},
		{table: "records", column: "type", wantType: "character varying"},
		{table: "records", column: "etag", wantType: "character varying"},
	}

	for _, spec := range specs {
		t.Run(spec.table+"/"+spec.column, func(t *testing.T) {
			var dataType string
			err := rawDB.Raw(
				`SELECT data_type FROM information_schema.columns
				 WHERE table_schema = 'public' AND table_name = ? AND column_name = ?`,
				spec.table, spec.column,
			).Scan(&dataType).Error

			if err != nil {
				t.Fatalf("query failed: %v", err)
			}
			if dataType == "" {
				t.Fatalf("column %s.%s not found in public schema", spec.table, spec.column)
			}
			if dataType != spec.wantType {
				t.Errorf("column %s.%s: want type %q, got %q",
					spec.table, spec.column, spec.wantType, dataType)
			}
		})
	}
}

// TestSchema_PrimaryKeys verifies composite primary keys on both tables.
func TestSchema_PrimaryKeys(t *testing.T) {
	type pkSpec struct {
		table       string
		wantColumns []string
	}

	specs := []pkSpec{
		{table: "results", wantColumns: []string{"id", "parent"}},
		{table: "records", wantColumns: []string{"id", "parent", "result_id"}},
	}

	for _, spec := range specs {
		t.Run(spec.table, func(t *testing.T) {
			var columns []string
			err := rawDB.Raw(`
				SELECT a.attname
				FROM   pg_index i
				JOIN   pg_attribute a ON a.attrelid = i.indrelid
				                     AND a.attnum = ANY(i.indkey)
				JOIN   pg_class c ON c.oid = i.indrelid
				JOIN   pg_namespace n ON n.oid = c.relnamespace
				WHERE  n.nspname = 'public'
				  AND  c.relname = ?
				  AND  i.indisprimary
				ORDER BY a.attname`,
				spec.table,
			).Scan(&columns).Error

			if err != nil {
				t.Fatalf("query failed: %v", err)
			}
			if len(columns) != len(spec.wantColumns) {
				t.Fatalf("want PK columns %v, got %v", spec.wantColumns, columns)
			}
			for i, want := range spec.wantColumns {
				if columns[i] != want {
					t.Errorf("PK column %d: want %q, got %q", i, want, columns[i])
				}
			}
		})
	}
}

// TestSchema_UniqueIndexes verifies unique indexes defined in GORM model tags
// exist and are actually unique in the live Postgres schema.
func TestSchema_UniqueIndexes(t *testing.T) {
	for _, idx := range []string{"results_by_name", "records_by_name"} {
		t.Run(idx, func(t *testing.T) {
			var isUnique bool
			err := rawDB.Raw(`
				SELECT i.indisunique
				FROM   pg_class c
				JOIN   pg_index i ON i.indexrelid = c.oid
				JOIN   pg_namespace n ON n.oid = c.relnamespace
				WHERE  n.nspname = 'public' AND c.relname = ?`,
				idx,
			).Row().Scan(&isUnique)

			if err != nil {
				t.Fatalf("index %q not found in public schema: %v", idx, err)
			}
			if !isUnique {
				t.Errorf("index %q exists but is not unique", idx)
			}
		})
	}
}

// TestSchema_ForeignKeyExists verifies the FK relationship from records → results.
func TestSchema_ForeignKeyExists(t *testing.T) {
	var count int64
	err := rawDB.Raw(`
		SELECT count(*)
		FROM   information_schema.table_constraints
		WHERE  table_schema = 'public'
		  AND  table_name = 'records'
		  AND  constraint_type = 'FOREIGN KEY'`,
	).Scan(&count).Error

	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if count == 0 {
		t.Error("expected at least one foreign key on the records table")
	}
}

// TestSchema_ForeignKeyCascade verifies that the FK from records → results
// uses ON DELETE CASCADE. The query filters by the specific referenced table
// to be deterministic if additional FKs are added later.
func TestSchema_ForeignKeyCascade(t *testing.T) {
	var deleteRule string
	err := rawDB.Raw(`
		SELECT rc.delete_rule
		FROM   information_schema.referential_constraints rc
		JOIN   information_schema.table_constraints tc
		  ON   rc.constraint_name = tc.constraint_name
		  AND  rc.constraint_schema = tc.constraint_schema
		JOIN   information_schema.constraint_table_usage ctu
		  ON   rc.unique_constraint_name = ctu.constraint_name
		  AND  rc.constraint_schema = ctu.constraint_schema
		WHERE  tc.table_schema = 'public'
		  AND  tc.table_name = 'records'
		  AND  ctu.table_name = 'results'`,
	).Scan(&deleteRule).Error

	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if deleteRule != "CASCADE" {
		t.Errorf("expected ON DELETE CASCADE for records→results FK, got %q", deleteRule)
	}
}

// TestSchema_JsonbQueryable verifies that jsonb columns support Postgres-native
// operators. The @> (containment) operator would fail on SQLite TEXT columns.
// This test inserts a row via the deployed API, then queries via raw DB.
func TestSchema_JsonbQueryable(t *testing.T) {
	ctx := defaultCtx()

	res, err := adminClient.CreateResult(ctx, &pb.CreateResultRequest{
		Parent: "db-schema-jsonb",
		Result: &pb.Result{
			Name:        "db-schema-jsonb/results/jsonb-query",
			Annotations: map[string]string{"env": "production", "team": "platform"},
		},
	})
	if err != nil {
		t.Fatalf("CreateResult failed: %v", err)
	}
	t.Cleanup(func() { deleteResult(t, res.GetName()) })

	var count int64
	err = rawDB.Raw(
		`SELECT count(*) FROM results WHERE annotations @> ?`,
		`{"env":"production"}`,
	).Scan(&count).Error
	if err != nil {
		t.Fatalf("jsonb containment query failed: %v", err)
	}
	if count == 0 {
		t.Error("expected jsonb @> query to find the inserted Result")
	}
}
