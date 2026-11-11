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

package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Structural validation compares the live catalog with the baseline schema
// (001_baseline.up.sql). It is used only where no version marker can be
// trusted: adopting a database created by GORM AutoMigrate during migrate-up,
// and as an explicit operator diagnostic. Application startup never runs it.
//
// Baseline objects must match exactly. Additions that existing writers cannot
// notice are accepted, so the check stays valid after additive migrations:
// extra tables, extra nullable or defaulted columns, and extra non-unique
// indexes. Migrations never add a unique index, constraint or required column
// to results or records (see README.md, "Adding a migration"), so these
// expectations stay frozen.

type column struct {
	Type    string
	NotNull bool
	Default string
}

type index struct {
	Columns string
	Primary bool
	Unique  bool
}

var baselineTables = []string{"results", "records"}

var baselineColumns = func() map[string]column {
	const (
		id   = "character varying(64)"
		ts   = "timestamp with time zone"
		now  = "CURRENT_TIMESTAMP"
		etag = "character varying(128)"
	)
	return map[string]column{
		"results.parent":                    {id, true, ""},
		"results.id":                        {id, true, ""},
		"results.name":                      {id, false, ""},
		"results.annotations":               {"jsonb", false, ""},
		"results.created_time":              {ts, false, now},
		"results.updated_time":              {ts, false, now},
		"results.recordsummary_record":      {"character varying(256)", false, ""},
		"results.recordsummary_type":        {"character varying(768)", false, ""},
		"results.recordsummary_start_time":  {ts, false, ""},
		"results.recordsummary_end_time":    {ts, false, ""},
		"results.recordsummary_status":      {"integer", false, ""},
		"results.recordsummary_annotations": {"jsonb", false, ""},
		"results.etag":                      {etag, false, ""},
		"records.parent":                    {id, true, ""},
		"records.result_id":                 {id, true, ""},
		"records.result_name":               {id, false, ""},
		"records.id":                        {id, true, ""},
		"records.name":                      {id, false, ""},
		"records.type":                      {"character varying(768)", false, ""},
		"records.data":                      {"jsonb", false, ""},
		"records.created_time":              {ts, false, now},
		"records.updated_time":              {ts, false, now},
		"records.etag":                      {etag, false, ""},
	}
}()

// Keyed by table and columns; index names other than the unique lookup indexes
// are not significant.
var baselineIndexes = map[string]index{
	"results(parent,id)":               {Columns: "parent,id", Primary: true, Unique: true},
	"results(parent,name)":             {Columns: "parent,name", Unique: true},
	"records(parent,result_id,id)":     {Columns: "parent,result_id,id", Primary: true, Unique: true},
	"records(parent,result_name,name)": {Columns: "parent,result_name,name", Unique: true},
}

const baselineForeignKey = "records(parent,result_id) REFERENCES results(parent,id) ON UPDATE CASCADE ON DELETE CASCADE"

const columnsQuery = `
SELECT c.relname, a.attname, format_type(a.atttypid, a.atttypmod), a.attnotnull,
       COALESCE(pg_get_expr(d.adbin, d.adrelid), '')
FROM pg_attribute a
JOIN pg_class c ON c.oid = a.attrelid
LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
WHERE c.relnamespace = current_schema()::regnamespace AND c.relkind = 'r'
  AND c.relname = ANY($1) AND a.attnum > 0 AND NOT a.attisdropped`

// Expression and partial indexes report no plain column list and never match
// a baseline index.
const indexesQuery = `
SELECT c.relname, ic.relname, i.indisprimary, i.indisunique,
       i.indisvalid AND i.indisready AND i.indislive,
       am.amname = 'btree' AND i.indexprs IS NULL AND i.indpred IS NULL,
       array_to_string(ARRAY(
         SELECT a.attname FROM unnest(i.indkey[0:i.indnkeyatts-1]) WITH ORDINALITY k(num, pos)
         JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = k.num ORDER BY k.pos), ',')
FROM pg_index i
JOIN pg_class c ON c.oid = i.indrelid
JOIN pg_class ic ON ic.oid = i.indexrelid
JOIN pg_am am ON am.oid = ic.relam
WHERE c.relnamespace = current_schema()::regnamespace AND c.relname = ANY($1)`

// Primary keys and unique constraints are validated through their indexes;
// NOT NULL constraints through columns.
const constraintsQuery = `
SELECT c.relname, k.conname, k.contype, k.convalidated,
       format('%s(%s) REFERENCES %s(%s) ON UPDATE %s ON DELETE %s',
         c.relname,
         array_to_string(ARRAY(SELECT a.attname FROM unnest(k.conkey) WITH ORDINALITY x(num, pos)
           JOIN pg_attribute a ON a.attrelid = k.conrelid AND a.attnum = x.num ORDER BY x.pos), ','),
         COALESCE(rc.relname, ''),
         array_to_string(ARRAY(SELECT a.attname FROM unnest(k.confkey) WITH ORDINALITY x(num, pos)
           JOIN pg_attribute a ON a.attrelid = k.confrelid AND a.attnum = x.num ORDER BY x.pos), ','),
         CASE k.confupdtype WHEN 'c' THEN 'CASCADE' ELSE k.confupdtype::text END,
         CASE k.confdeltype WHEN 'c' THEN 'CASCADE' ELSE k.confdeltype::text END)
FROM pg_constraint k
JOIN pg_class c ON c.oid = k.conrelid
LEFT JOIN pg_class rc ON rc.oid = k.confrelid
WHERE c.relnamespace = current_schema()::regnamespace AND c.relname = ANY($1)
  AND k.contype NOT IN ('p', 'u', 'n')`

// ValidateStructure checks that the baseline Results schema is present and
// unmodified in the connection's current schema. The error lists every
// difference.
func ValidateStructure(ctx context.Context, q Queryer) error {
	var diffs []string
	diff := func(format string, args ...any) { diffs = append(diffs, fmt.Sprintf(format, args...)) }

	found := map[string]bool{}
	wantColumns := map[string]column{}
	for k, v := range baselineColumns {
		wantColumns[k] = v
	}
	err := eachRow(ctx, q, columnsQuery, func(rows *sql.Rows) error {
		var table, name string
		var got column
		if err := rows.Scan(&table, &name, &got.Type, &got.NotNull, &got.Default); err != nil {
			return err
		}
		found[table] = true
		key := table + "." + name
		want, ok := wantColumns[key]
		switch {
		case !ok && got.NotNull && got.Default == "":
			diff("%s: unexpected NOT NULL column without default", key)
		case ok && got != want:
			diff("%s: got %s, want %s", key, got, want)
		}
		delete(wantColumns, key)
		return nil
	})
	if err != nil {
		return fmt.Errorf("read schema columns: %w", err)
	}
	for _, table := range baselineTables {
		if !found[table] {
			diff("%s: missing table", table)
		}
	}
	for key := range wantColumns {
		if found[strings.SplitN(key, ".", 2)[0]] {
			diff("%s: missing column", key)
		}
	}

	wantIndexes := map[string]index{}
	for k, v := range baselineIndexes {
		wantIndexes[k] = v
	}
	err = eachRow(ctx, q, indexesQuery, func(rows *sql.Rows) error {
		var table, name string
		var usable, plain bool
		var got index
		if err := rows.Scan(&table, &name, &got.Primary, &got.Unique, &usable, &plain, &got.Columns); err != nil {
			return err
		}
		key := table + "(" + got.Columns + ")"
		want, ok := wantIndexes[key]
		switch {
		case ok && plain && usable && got == want:
			delete(wantIndexes, key)
		case ok && got.Unique:
			diff("index %s on %s: want a valid plain btree %s", name, key, want)
		case got.Unique:
			diff("index %s on %s: unexpected unique index", name, key)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("read schema indexes: %w", err)
	}
	for key, want := range wantIndexes {
		if found[strings.SplitN(key, "(", 2)[0]] {
			diff("%s: missing %s", key, want)
		}
	}

	foreignKey := false
	err = eachRow(ctx, q, constraintsQuery, func(rows *sql.Rows) error {
		var table, name, kind, definition string
		var validated bool
		if err := rows.Scan(&table, &name, &kind, &validated, &definition); err != nil {
			return err
		}
		if kind == "f" && definition == baselineForeignKey && validated && !foreignKey {
			foreignKey = true
			return nil
		}
		diff("constraint %s on %s: unexpected (type %s, validated %t)", name, table, kind, validated)
		return nil
	})
	if err != nil {
		return fmt.Errorf("read schema constraints: %w", err)
	}
	if !foreignKey && found["records"] && found["results"] {
		diff("missing foreign key %s", baselineForeignKey)
	}

	if len(diffs) == 0 {
		return nil
	}
	slices.Sort(diffs)
	return fmt.Errorf("schema does not match the Results baseline:\n  %s", strings.Join(diffs, "\n  "))
}

// eachRow runs a catalog query over the baseline tables and calls scan for
// every row.
func eachRow(ctx context.Context, q Queryer, query string, scan func(*sql.Rows) error) (err error) {
	rows, err := q.QueryContext(ctx, query, baselineTables)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (c column) String() string {
	s := c.Type
	if c.NotNull {
		s += " NOT NULL"
	}
	if c.Default != "" {
		s += " DEFAULT " + c.Default
	}
	return s
}

func (i index) String() string {
	kind := "index"
	switch {
	case i.Primary:
		kind = "primary key"
	case i.Unique:
		kind = "unique index"
	}
	return kind + " (" + i.Columns + ")"
}
