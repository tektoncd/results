//go:build e2e

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
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	resultsdb "github.com/tektoncd/results/pkg/api/server/db"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// These tests need a PostgreSQL server where DB_URL's user may create
// databases, for example:
//
//	DB_URL="host=localhost port=5432 user=postgres password=postgres dbname=postgres sslmode=disable" \
//	  go test -tags=e2e ./pkg/api/server/db/migrations/
//
// Every test runs in its own temporary database.

var opts = Options{ConnectTimeout: 10 * time.Second, LockTimeout: 10 * time.Second}

func tempDB(t *testing.T) (string, *sql.DB) {
	t.Helper()
	base := os.Getenv("DB_URL")
	if base == "" {
		t.Skip("DB_URL not set")
	}
	cfg, err := pgx.ParseConfig(base)
	if err != nil {
		t.Fatal(err)
	}
	admin := stdlib.OpenDB(*cfg)
	t.Cleanup(func() { admin.Close() })
	name := fmt.Sprintf("results_migrations_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)"); err != nil {
			t.Errorf("drop %s: %v", name, err)
		}
	})

	var dsn string
	if u, err := url.Parse(base); err == nil && (u.Scheme == "postgres" || u.Scheme == "postgresql") {
		u.Path = "/" + name
		dsn = u.String()
	} else {
		dsn = base + " dbname=" + name
	}
	db, err := Open(dsn, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return dsn, db
}

func exec(t *testing.T, db *sql.DB, stmts ...string) {
	t.Helper()
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

func wantErr(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), substr) {
		t.Fatalf("error = %v, want containing %q", err, substr)
	}
}

// gormSchema creates the schema the way releases before versioned migrations
// did.
func gormSchema(t *testing.T, dsn string) {
	t.Helper()
	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	if err := gdb.AutoMigrate(&resultsdb.Result{}, &resultsdb.Record{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := gdb.DB()
	sqlDB.Close()
}

func TestConnectDoesNotRetryRejectedLogin(t *testing.T) {
	dsn, _ := tempDB(t)
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	bad := fmt.Sprintf("host=%s port=%d user=%s password=wrong-%d dbname=%s sslmode=disable",
		cfg.Host, cfg.Port, cfg.User, time.Now().UnixNano(), cfg.Database)
	o := opts
	o.WaitTimeout = time.Minute
	start := time.Now()
	_, err = Connect(context.Background(), bad, o)
	if err == nil {
		t.Fatal("Connect() with a wrong password succeeded")
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("Connect() retried a rejected login for %s", elapsed)
	}
}

func TestMigrateUp(t *testing.T) {
	ctx := context.Background()
	dsn, db := tempDB(t)

	_, err := Check(ctx, db, RequiredSchemaVersion)
	wantErr(t, err, "run `results-admin migrate-up`")

	latest, err := Latest()
	if err != nil {
		t.Fatal(err)
	}
	s, err := MigrateUp(ctx, dsn, opts)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Status{Initialized: true, Version: int(latest)}); s != want {
		t.Fatalf("status = %+v, want %+v", s, want)
	}
	if _, err := MigrateUp(ctx, dsn, opts); err != nil {
		t.Fatalf("second migrate-up: %v", err)
	}
	if _, err := Check(ctx, db, RequiredSchemaVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := Check(ctx, db, latest+1); err == nil {
		t.Fatal("Check accepted a version newer than the schema")
	}
	_, err = MigrateTo(ctx, dsn, opts, latest-1)
	if latest > 1 {
		wantErr(t, err, "downgrades are not supported")
	}
}

// TestBaselineStructure keeps the hand-coded expectations in structure.go in
// step with the baseline migration and with the GORM models.
func TestBaselineStructure(t *testing.T) {
	ctx := context.Background()
	t.Run("baseline migration", func(t *testing.T) {
		dsn, db := tempDB(t)
		if _, err := MigrateTo(ctx, dsn, opts, 1); err != nil {
			t.Fatal(err)
		}
		if err := ValidateStructure(ctx, db); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("gorm models", func(t *testing.T) {
		dsn, db := tempDB(t)
		gormSchema(t, dsn)
		if err := ValidateStructure(ctx, db); err != nil {
			t.Fatal(err)
		}
	})
}

func TestValidateStructureDrift(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		stmts []string
		want  string
	}{
		{"missing table", []string{"DROP TABLE records"}, "records: missing table"},
		{"missing column", []string{"ALTER TABLE records DROP COLUMN etag"}, "records.etag: missing column"},
		{"changed type", []string{"ALTER TABLE results ALTER COLUMN name TYPE text"}, "results.name: got text"},
		{"dropped default", []string{"ALTER TABLE results ALTER COLUMN created_time DROP DEFAULT"}, "results.created_time"},
		{"required extra column", []string{"ALTER TABLE results ADD COLUMN x int NOT NULL"}, "results.x: unexpected NOT NULL"},
		{"missing unique index", []string{"DROP INDEX results_by_name"}, "results(parent,name): missing unique index"},
		{"non-unique lookup index", []string{"DROP INDEX records_by_name", "CREATE INDEX records_by_name ON records(parent, result_name, name)"}, "records(parent,result_name,name): missing unique index"},
		{"partial unique index", []string{"DROP INDEX results_by_name", "CREATE UNIQUE INDEX results_by_name ON results(parent, name) WHERE name <> ''"}, "want a valid plain btree"},
		{"extra unique index", []string{"CREATE UNIQUE INDEX x ON records(name)"}, "unexpected unique index"},
		{"missing foreign key", []string{"ALTER TABLE records DROP CONSTRAINT fk_records_result"}, "missing foreign key"},
		{"changed foreign key", []string{"ALTER TABLE records DROP CONSTRAINT fk_records_result", "ALTER TABLE records ADD CONSTRAINT fk FOREIGN KEY (parent, result_id) REFERENCES results(parent, id)"}, "constraint fk on records"},
		{"check constraint", []string{"ALTER TABLE records ADD CONSTRAINT c CHECK (name <> '')"}, "constraint c on records"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dsn, db := tempDB(t)
			gormSchema(t, dsn)
			exec(t, db, tc.stmts...)
			wantErr(t, ValidateStructure(ctx, db), tc.want)
		})
	}

	t.Run("compatible additions", func(t *testing.T) {
		dsn, db := tempDB(t)
		gormSchema(t, dsn)
		exec(t, db,
			"CREATE TABLE extra (id int PRIMARY KEY)",
			"ALTER TABLE records ADD COLUMN optional text",
			"ALTER TABLE records ADD COLUMN defaulted int NOT NULL DEFAULT 0",
			"CREATE INDEX extra_idx ON records(type)",
			"CREATE INDEX extra_dup ON results(parent, name)",
		)
		if err := ValidateStructure(ctx, db); err != nil {
			t.Fatal(err)
		}
	})
}

func TestMigrateUpAdoptsExisting(t *testing.T) {
	ctx := context.Background()
	dsn, db := tempDB(t)
	gormSchema(t, dsn)
	exec(t, db,
		"INSERT INTO results (parent, id, name) VALUES ('ns', '1', 'r')",
		"INSERT INTO records (parent, result_id, result_name, id, name, data) VALUES ('ns', '1', 'r', '2', 'rec', '{}')",
	)
	_, err := Check(ctx, db, RequiredSchemaVersion)
	wantErr(t, err, "to adopt and migrate it")

	latest, err := Latest()
	if err != nil {
		t.Fatal(err)
	}
	s, err := MigrateUp(ctx, dsn, opts)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Status{Initialized: true, Version: int(latest)}); s != want {
		t.Fatalf("status = %+v, want %+v", s, want)
	}
	if _, err := Check(ctx, db, RequiredSchemaVersion); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow("SELECT count(*) FROM records").Scan(&n); err != nil || n != 1 {
		t.Fatalf("records = %d, %v; want existing data kept", n, err)
	}

	// An API server of the earlier release starting afterwards runs GORM
	// AutoMigrate; it must leave the adopted schema valid.
	gormSchema(t, dsn)
	if err := ValidateStructure(ctx, db); err != nil {
		t.Fatalf("schema after earlier-release AutoMigrate: %v", err)
	}
}

func TestMigrateUpRejectsDriftedExisting(t *testing.T) {
	ctx := context.Background()
	dsn, db := tempDB(t)
	gormSchema(t, dsn)
	exec(t, db, "DROP INDEX records_by_name")
	_, err := MigrateUp(ctx, dsn, opts)
	wantErr(t, err, "cannot adopt the existing Results tables")
	wantErr(t, err, "records(parent,result_name,name): missing unique index")
	s, err := ReadStatus(ctx, db)
	if err != nil || s.Initialized {
		t.Fatalf("status = %+v, %v; want uninitialized", s, err)
	}
}

func TestCheckDirty(t *testing.T) {
	ctx := context.Background()
	dsn, db := tempDB(t)
	if _, err := MigrateUp(ctx, dsn, opts); err != nil {
		t.Fatal(err)
	}
	latest, _ := Latest()

	// A migration to latest+1 that is running or failed.
	exec(t, db, fmt.Sprintf("UPDATE %s SET version = %d, dirty = true", migrationsTable, latest+1))
	if _, err := Check(ctx, db, latest); err != nil {
		t.Fatalf("dirty newer version must serve code requiring the previous one: %v", err)
	}
	_, err := Check(ctx, db, latest+1)
	wantErr(t, err, "is dirty")
	_, err = MigrateUp(ctx, dsn, opts)
	wantErr(t, err, "is dirty")

	_, err = Force(ctx, dsn, opts, latest+1)
	wantErr(t, err, "version must be between")
	if latest > 1 {
		_, err = Force(ctx, dsn, opts, latest-1)
		wantErr(t, err, "version must be")
	}
	s, err := Force(ctx, dsn, opts, latest)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Status{Initialized: true, Version: int(latest)}); s != want {
		t.Fatalf("status = %+v, want %+v", s, want)
	}
	_, err = Force(ctx, dsn, opts, latest)
	wantErr(t, err, "not dirty")
}

func TestReadStatusMultipleRows(t *testing.T) {
	ctx := context.Background()
	dsn, db := tempDB(t)
	if _, err := MigrateUp(ctx, dsn, opts); err != nil {
		t.Fatal(err)
	}
	exec(t, db, fmt.Sprintf("INSERT INTO %s VALUES (99, false)", migrationsTable))
	_, err := Check(ctx, db, RequiredSchemaVersion)
	wantErr(t, err, "more than one version row")
}

func TestLockTimeout(t *testing.T) {
	ctx := context.Background()
	dsn, _ := tempDB(t)

	// Hold the migration lock as a concurrent results-admin would.
	holderDB, err := Open(dsn, opts)
	if err != nil {
		t.Fatal(err)
	}
	holder, err := migratepgx.WithInstance(holderDB, &migratepgx.Config{MigrationsTable: migrationsTable})
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	if err := holder.Lock(); err != nil {
		t.Fatal(err)
	}

	short := opts
	short.LockTimeout = 500 * time.Millisecond
	start := time.Now()
	_, err = MigrateUp(ctx, dsn, short)
	wantErr(t, err, "timed out waiting for the migration lock")
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("lock wait took %s, want bounded by lock_timeout", d)
	}

	if err := holder.Unlock(); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateUp(ctx, dsn, opts); err != nil {
		t.Fatalf("migrate-up after the lock is released: %v", err)
	}
}
