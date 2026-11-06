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

// Package migrations owns the versioned PostgreSQL schema of Tekton Results.
//
// Schema changes are embedded SQL files applied by the results-admin binary.
// Application processes only read the recorded version with Check; they never
// change the schema. Every migration must be additive and correct when applied
// from any earlier version in a single run, while older application versions
// keep serving. See docs/database-migrations.md.
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"
)

// RequiredSchemaVersion is the minimum schema this release's application code
// can run against. Raise it only when code starts depending on an object a
// migration added. Not every migration requires a bump.
const RequiredSchemaVersion uint = 1

// Published migration files are immutable; add a new migration instead of
// editing one.
//
//go:embed *.sql
var files embed.FS

// migrationsTable is the golang-migrate default history table, resolved through
// the connection's search_path like the Results tables themselves.
const migrationsTable = "schema_migrations"

// Queryer is implemented by *sql.DB, *sql.Conn and *sql.Tx.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Status is the migration history recorded in the database.
type Status struct {
	// Initialized is false when the database has no migration history.
	Initialized bool `json:"initialized"`
	// Version is the recorded version. golang-migrate records the target
	// version, marked dirty, before running a migration's SQL.
	// The type matches golang-migrate's Go API (int), not the database column (bigint).
	Version int `json:"version"`
	// Dirty is true while a migration runs, or after it failed.
	Dirty bool `json:"dirty"`
}

// Effective returns the highest version whose objects are known to be fully in
// place. A dirty version V means migration V started but did not complete, so
// migrations up to V-1 are complete. For a down migration the dirty row holds
// the down target, which makes this conservative as well.
func (s Status) Effective() uint {
	v := s.Version
	if s.Dirty {
		v--
	}
	if !s.Initialized || v < 0 {
		return 0
	}
	return uint(v)
}

// ReadStatus reads the migration history using only SELECT statements.
func ReadStatus(ctx context.Context, q Queryer) (s Status, err error) {
	var exists bool
	if err := q.QueryRowContext(ctx, "SELECT to_regclass($1) IS NOT NULL", migrationsTable).Scan(&exists); err != nil {
		return s, fmt.Errorf("read schema migration history: %w", err)
	}
	if !exists {
		return s, nil
	}
	rows, err := q.QueryContext(ctx, "SELECT version, dirty FROM "+migrationsTable)
	if err != nil {
		return s, fmt.Errorf("read schema migration history: %w", err)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	for rows.Next() {
		if s.Initialized {
			return s, errors.New("invalid schema migration history: more than one version row")
		}
		if err := rows.Scan(&s.Version, &s.Dirty); err != nil {
			return s, fmt.Errorf("read schema migration history: %w", err)
		}
		s.Initialized = true
	}
	if err := rows.Err(); err != nil {
		return s, fmt.Errorf("read schema migration history: %w", err)
	}
	return s, nil
}

// RequiredVersion returns the schema version the application requires: the
// override when set (non-zero), otherwise RequiredSchemaVersion.
func RequiredVersion(override uint) uint {
	if override != 0 {
		return override
	}
	return RequiredSchemaVersion
}

// Check verifies that the database can serve application code requiring the
// given schema version. It runs read-only queries and never changes the schema.
func Check(ctx context.Context, db *sql.DB, required uint) (Status, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Status{}, fmt.Errorf("check database schema: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // read-only transaction
	s, err := ReadStatus(ctx, tx)
	if err != nil {
		return s, err
	}
	if !s.Initialized {
		var legacy bool
		if err := tx.QueryRowContext(ctx, "SELECT to_regclass('results') IS NOT NULL").Scan(&legacy); err != nil {
			return s, fmt.Errorf("check database schema: %w", err)
		}
		if legacy {
			return s, errors.New("database has Results tables but no schema migration history; run `results-admin migrate-up` of this release to adopt and migrate it")
		}
		return s, errors.New("database has no schema migration history; run `results-admin migrate-up`")
	}
	return s, compatible(s, required)
}

func compatible(s Status, required uint) error {
	if s.Effective() >= required {
		return nil
	}
	if s.Dirty {
		return fmt.Errorf("schema version %d is dirty (a migration is running or failed) and this release requires version %d; wait for `results-admin migrate-up` to finish, or repair the failed migration", s.Version, required)
	}
	return fmt.Errorf("schema version %d is older than version %d required by this release; run `results-admin migrate-up` of this release", s.Version, required)
}

// StartupCheckTimeout bounds the schema check performed at process startup.
const StartupCheckTimeout = 30 * time.Second

// CheckAtStartup runs Check for a process whose configuration may override the
// required version. A non-zero override is reported through warnf because it
// disables the protection this release's own requirement provides.
func CheckAtStartup(ctx context.Context, db *sql.DB, override uint, warnf func(string, ...any)) (Status, error) {
	required := RequiredVersion(override)
	if required != RequiredSchemaVersion {
		warnf("DB_SCHEMA_REQUIRED_VERSION_OVERRIDE is set: requiring database schema version %d instead of version %d required by this release", required, RequiredSchemaVersion)
	}
	ctx, cancel := context.WithTimeout(ctx, StartupCheckTimeout)
	defer cancel()
	return Check(ctx, db, required)
}
