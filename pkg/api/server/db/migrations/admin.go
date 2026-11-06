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
	"io"
	"io/fs"
	"math"
	"net"
	"strconv"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
)

// Options bounds an administrative operation. Zero values disable a limit.
type Options struct {
	// ConnectTimeout bounds establishing each database connection.
	ConnectTimeout time.Duration
	// LockTimeout bounds every lock wait, including the migration advisory lock
	// and table locks taken by DDL (PostgreSQL lock_timeout).
	LockTimeout time.Duration
	// StatementTimeout bounds each statement (PostgreSQL statement_timeout).
	StatementTimeout time.Duration
	// WaitTimeout is how long Connect keeps retrying while the database is
	// not reachable yet, for example while it is still starting.
	WaitTimeout time.Duration
	// Reload, if set, returns the current connection settings. Connect calls
	// it before each retry while waiting for the database.
	Reload func() (string, error)
	// Logf, if set, reports each failed attempt while waiting for the database.
	Logf func(format string, args ...any)
}

// Open connects to PostgreSQL for administrative operations, applying the
// limits in o to every session.
func Open(dsn string, o Options) (*sql.DB, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database connection settings: %w", err)
	}
	if o.ConnectTimeout > 0 {
		cfg.ConnectTimeout = o.ConnectTimeout
	}
	cfg.RuntimeParams["lock_timeout"] = strconv.FormatInt(o.LockTimeout.Milliseconds(), 10)
	cfg.RuntimeParams["statement_timeout"] = strconv.FormatInt(o.StatementTimeout.Milliseconds(), 10)
	return stdlib.OpenDB(*cfg), nil
}

// Connect opens the database like Open and waits until it accepts
// connections. Errors showing that the server is not reachable yet are
// retried for up to o.WaitTimeout; any other error is returned immediately.
// If o.Reload is set, the connection settings are reloaded before each retry,
// so a configuration change made while waiting takes effect.
func Connect(ctx context.Context, dsn string, o Options) (*sql.DB, error) {
	deadline := time.Now().Add(o.WaitTimeout)
	delay := time.Second
	for {
		db, err := Open(dsn, o)
		if err != nil {
			return nil, err
		}
		err = db.PingContext(ctx)
		if err == nil {
			return db, nil
		}
		if cerr := db.Close(); cerr != nil {
			err = errors.Join(err, cerr)
		}
		if !notReachable(err) || ctx.Err() != nil {
			return nil, fmt.Errorf("connect to database: %w", err)
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			if o.WaitTimeout > 0 {
				return nil, fmt.Errorf("connect to database: still not reachable after %s: %w", o.WaitTimeout, err)
			}
			return nil, fmt.Errorf("connect to database: %w", err)
		}
		wait := min(delay, remaining)
		o.logf("database not reachable, retrying in %s: %v", wait, err)
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("connect to database: %w", err)
		case <-time.After(wait):
		}
		delay = min(2*delay, 10*time.Second)
		if o.Reload != nil {
			next, err := o.Reload()
			if err != nil {
				return nil, fmt.Errorf("reload database settings: %w", err)
			}
			if next != dsn {
				o.logf("database settings changed, using the new settings")
				dsn = next
			}
		}
	}
}

func (o Options) logf(format string, args ...any) {
	if o.Logf != nil {
		o.Logf(format, args...)
	}
}

// notReachable reports whether err means the server could not be reached or
// is not accepting connections yet, as opposed to rejecting the request.
func notReachable(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgerrcode.CannotConnectNow || pgErr.Code == pgerrcode.TooManyConnections
	}
	var netErr net.Error
	return errors.As(err, &netErr) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

// Latest returns the highest embedded migration version.
func Latest() (latest uint, err error) {
	src, err := iofs.New(files, ".")
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, src.Close()) }()
	v, err := src.First()
	if err != nil {
		return 0, fmt.Errorf("read embedded migrations: %w", err)
	}
	for {
		next, err := src.Next(v)
		if errors.Is(err, fs.ErrNotExist) {
			return v, nil
		}
		if err != nil {
			return 0, fmt.Errorf("read embedded migrations: %w", err)
		}
		v = next
	}
}

// MigrateUp applies all pending embedded migrations.
func MigrateUp(ctx context.Context, dsn string, o Options) (Status, error) {
	return migrateTo(ctx, dsn, o, 0)
}

// MigrateTo applies embedded migrations up to target. It never downgrades.
func MigrateTo(ctx context.Context, dsn string, o Options, target uint) (Status, error) {
	if target == 0 {
		return Status{}, errors.New("target version must be at least 1")
	}
	return migrateTo(ctx, dsn, o, target)
}

// migrateTo migrates to target, or to the latest version when target is 0.
func migrateTo(ctx context.Context, dsn string, o Options, target uint) (Status, error) {
	return withLock(ctx, dsn, o, func(db *sql.DB, drv database.Driver) (err error) {
		s, err := ReadStatus(ctx, db)
		if err != nil {
			return err
		}
		if !s.Initialized {
			if err := adopt(ctx, db, drv); err != nil {
				return err
			}
		}
		if s.Dirty {
			return fmt.Errorf("schema version %d is dirty: a migration failed or is still running; repair it before migrating", s.Version)
		}
		if target != 0 && target < uint(s.Version) { //nolint:gosec // migration versions are small
			return fmt.Errorf("target version %d is below current version %d; downgrades are not supported", target, s.Version)
		}

		src, err := iofs.New(files, ".")
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, src.Close()) }()
		m, err := migrate.NewWithInstance("iofs", src, "pgx5", heldLock{drv})
		if err != nil {
			return err
		}
		// Stop between migrations on cancellation; a running statement is
		// bounded by statement_timeout.
		done := make(chan struct{})
		defer close(done)
		go func() {
			select {
			case <-ctx.Done():
				m.GracefulStop <- true
			case <-done:
			}
		}()
		if target == 0 {
			err = m.Up()
		} else {
			err = m.Migrate(target)
		}
		if err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("migration failed: %w", err)
		}
		return ctx.Err()
	})
}

// adopt records version 1 for a database created by a release that managed
// the schema with GORM AutoMigrate, provided its structure matches the
// baseline exactly. Existing data is not changed. API servers of such a
// release may keep running: their AutoMigrate finds the baseline complete and
// changes nothing, and later migrations leave baseline objects untouched.
func adopt(ctx context.Context, db *sql.DB, drv database.Driver) error {
	legacy, err := hasResultsTables(ctx, db)
	if err != nil || !legacy {
		return err
	}
	if err := ValidateStructure(ctx, db); err != nil {
		return fmt.Errorf("cannot adopt the existing Results tables: %w", err)
	}
	return drv.SetVersion(1, false)
}

// Force records version as clean on a dirty database, after an administrator
// has repaired a failed migration by hand: either completed it (version is the
// failed migration) or undone its partial changes (version is the one before).
// It changes no schema objects.
func Force(ctx context.Context, dsn string, o Options, version uint) (Status, error) {
	latest, err := Latest()
	if err != nil {
		return Status{}, err
	}
	if version == 0 || version > latest {
		return Status{}, fmt.Errorf("version must be between 1 and %d", latest)
	}
	return withLock(ctx, dsn, o, func(db *sql.DB, drv database.Driver) error {
		s, err := ReadStatus(ctx, db)
		if err != nil {
			return err
		}
		if !s.Dirty {
			return errors.New("schema is not dirty; force only repairs a failed migration")
		}
		// Ensure version fits in int (which may be 32-bit on some platforms).
		// In practice Latest() already limits this, but CodeQL needs an explicit check.
		if version > math.MaxInt32 {
			return fmt.Errorf("version %d exceeds maximum supported value", version)
		}
		v := int(version)
		if v != s.Version && v != s.Version-1 {
			return fmt.Errorf("version must be %d (failed migration completed by hand) or %d (its changes undone)", s.Version, s.Version-1)
		}
		return drv.SetVersion(v, false)
	})
}

// withLock runs fn while holding the golang-migrate advisory lock, then returns
// the resulting status. Creating the driver creates the empty history table if
// it is missing, which ReadStatus still reports as uninitialized.
func withLock(ctx context.Context, dsn string, o Options, fn func(*sql.DB, database.Driver) error) (s Status, err error) {
	db, err := Connect(ctx, dsn, o)
	if err != nil {
		return s, err
	}
	// drv.Close also closes db.
	drv, err := migratepgx.WithInstance(db, &migratepgx.Config{MigrationsTable: migrationsTable})
	if err != nil {
		return s, errors.Join(lockError("initialize migration driver", err), db.Close())
	}
	defer func() { err = errors.Join(err, drv.Close()) }()
	if err := drv.Lock(); err != nil {
		return s, lockError("acquire migration lock", err)
	}
	defer func() { err = errors.Join(err, drv.Unlock()) }()
	err = fn(db, drv)
	// Report the state after failures too: the dirty flag guides recovery.
	s, statusErr := ReadStatus(context.WithoutCancel(ctx), db)
	return s, errors.Join(err, statusErr)
}

// lockError explains lock_timeout expiry, which golang-migrate wraps without
// Unwrap.
func lockError(op string, err error) error {
	var pgErr *pgconn.PgError
	var dbErr *database.Error
	if errors.As(err, &dbErr) && errors.As(dbErr.OrigErr, &pgErr) && pgErr.Code == pgerrcode.LockNotAvailable {
		return fmt.Errorf("%s: timed out waiting for the migration lock; another migration may be running: %w", op, err)
	}
	return fmt.Errorf("%s: %w", op, err)
}

func hasResultsTables(ctx context.Context, q Queryer) (bool, error) {
	var exists bool
	err := q.QueryRowContext(ctx, "SELECT to_regclass('results') IS NOT NULL OR to_regclass('records') IS NOT NULL").Scan(&exists)
	return exists, err
}

// heldLock lets migrate.Migrate run inside a lock its caller already holds, so
// the state checks and the migration form one critical section.
type heldLock struct{ database.Driver }

func (heldLock) Lock() error   { return nil }
func (heldLock) Unlock() error { return nil }
