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
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

var fileName = regexp.MustCompile(`^(\d+)_([a-z0-9_]+)\.(up|down)\.sql$`)

// TestEmbeddedMigrations enforces the layout golang-migrate relies on: every
// version has exactly one up and one down file and versions are contiguous
// from 1.
func TestEmbeddedMigrations(t *testing.T) {
	entries, err := files.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	type pair struct{ name, up, down string }
	versions := map[uint64]*pair{}
	var highest uint64
	for _, e := range entries {
		m := fileName.FindStringSubmatch(e.Name())
		if m == nil {
			t.Errorf("%s: name must match %s", e.Name(), fileName)
			continue
		}
		v, err := strconv.ParseUint(m[1], 10, 32)
		if err != nil || v == 0 {
			t.Errorf("%s: version must be a positive integer", e.Name())
			continue
		}
		p := versions[v]
		if p == nil {
			p = &pair{name: m[2]}
			versions[v] = p
		}
		if p.name != m[2] {
			t.Errorf("version %d has files with different names: %q and %q", v, p.name, m[2])
		}
		body, err := files.ReadFile(e.Name())
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(body)) == "" {
			t.Errorf("%s: empty migration", e.Name())
		}
		if m[3] == "up" {
			p.up = e.Name()
		} else {
			p.down = e.Name()
		}
		highest = max(highest, v)
	}
	for v := uint64(1); v <= highest; v++ {
		p := versions[v]
		switch {
		case p == nil:
			t.Errorf("version %d: missing migration (versions must be contiguous)", v)
		case p.up == "":
			t.Errorf("version %d: missing up migration", v)
		case p.down == "":
			t.Errorf("version %d: missing down migration", v)
		}
	}
	if len(versions) != int(highest) {
		t.Errorf("duplicate versions with different zero padding")
	}

	latest, err := Latest()
	if err != nil {
		t.Fatal(err)
	}
	if uint64(latest) != highest {
		t.Errorf("Latest() = %d, want %d", latest, highest)
	}
	if RequiredSchemaVersion < 1 || RequiredSchemaVersion > latest {
		t.Errorf("RequiredSchemaVersion = %d, must be between 1 and the latest migration %d", RequiredSchemaVersion, latest)
	}
}

func TestEffective(t *testing.T) {
	for _, tc := range []struct {
		s    Status
		want uint
	}{
		{Status{}, 0},
		{Status{Initialized: true, Version: -1}, 0},
		{Status{Initialized: true, Version: 1}, 1},
		{Status{Initialized: true, Version: 1, Dirty: true}, 0},
		{Status{Initialized: true, Version: 5}, 5},
		{Status{Initialized: true, Version: 5, Dirty: true}, 4},
	} {
		if got := tc.s.Effective(); got != tc.want {
			t.Errorf("%+v.Effective() = %d, want %d", tc.s, got, tc.want)
		}
	}
}

func TestCompatible(t *testing.T) {
	for _, tc := range []struct {
		name     string
		s        Status
		required uint
		wantErr  string
	}{
		{"equal", Status{Initialized: true, Version: 3}, 3, ""},
		{"newer schema", Status{Initialized: true, Version: 7}, 3, ""},
		{"older schema", Status{Initialized: true, Version: 2}, 3, "older than version 3"},
		{"dirty above required", Status{Initialized: true, Version: 4, Dirty: true}, 3, ""},
		{"dirty at required", Status{Initialized: true, Version: 3, Dirty: true}, 3, "is dirty"},
		{"dirty below required", Status{Initialized: true, Version: 2, Dirty: true}, 3, "is dirty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := compatible(tc.s, tc.required)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Errorf("unexpected error: %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Errorf("error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestRequiredVersion(t *testing.T) {
	if got := RequiredVersion(0); got != RequiredSchemaVersion {
		t.Errorf("RequiredVersion(0) = %d, want %d", got, RequiredSchemaVersion)
	}
	if got := RequiredVersion(42); got != 42 {
		t.Errorf("RequiredVersion(42) = %d, want 42", got)
	}
}

func TestNotReachable(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{&net.OpError{Op: "dial", Err: errors.New("connection refused")}, true},
		{fmt.Errorf("wrapped: %w", &net.DNSError{Err: "no such host", IsNotFound: true}), true},
		{io.ErrUnexpectedEOF, true},
		{&pgconn.PgError{Code: pgerrcode.CannotConnectNow}, true},
		{&pgconn.PgError{Code: pgerrcode.TooManyConnections}, true},
		{&pgconn.PgError{Code: pgerrcode.InvalidPassword}, false},
		{&pgconn.PgError{Code: pgerrcode.InvalidCatalogName}, false},
		{errors.New("tls: failed to verify certificate"), false},
	} {
		if got := notReachable(tc.err); got != tc.want {
			t.Errorf("notReachable(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestConnectWaitsForUnreachableDatabase(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().(*net.TCPAddr)
	l.Close() // nothing listens on the port any more

	var attempts int
	o := Options{
		ConnectTimeout: time.Second,
		WaitTimeout:    1500 * time.Millisecond,
		Logf:           func(string, ...any) { attempts++ },
	}
	dsn := fmt.Sprintf("host=127.0.0.1 port=%d user=u password=p dbname=d sslmode=disable", addr.Port)
	start := time.Now()
	_, err = Connect(context.Background(), dsn, o)
	if err == nil || !strings.Contains(err.Error(), "still not reachable after 1.5s") {
		t.Fatalf("Connect() error = %v, want timeout", err)
	}
	if elapsed := time.Since(start); elapsed < o.WaitTimeout {
		t.Errorf("Connect() returned after %s, want at least %s", elapsed, o.WaitTimeout)
	}
	if attempts == 0 {
		t.Error("Connect() did not report any retry")
	}

	o.WaitTimeout = 0
	if _, err := Connect(context.Background(), dsn, o); err == nil || strings.Contains(err.Error(), "still not reachable") {
		t.Errorf("Connect() without wait error = %v, want the connection error", err)
	}
}

func TestConnectReloadsSettings(t *testing.T) {
	// A server that rejects every startup message: reachable, but the login
	// fails, so Connect must stop retrying once it uses this address.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			buf := make([]byte, 8192)
			_, _ = c.Read(buf)
			msg := "SFATAL\x00C28P01\x00Mpassword authentication failed\x00\x00"
			b := []byte{'E', 0, 0, 0, 0}
			n := len(msg) + 4
			b[1], b[2], b[3], b[4] = byte(n>>24), byte(n>>16), byte(n>>8), byte(n)
			_, _ = c.Write(append(b, msg...))
			c.Close()
		}
	}()
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedPort := closed.Addr().(*net.TCPAddr).Port
	closed.Close()

	dsnFor := func(port int) string {
		return fmt.Sprintf("host=127.0.0.1 port=%d user=u password=p dbname=d sslmode=disable", port)
	}
	var reloads int
	var logs []string
	o := Options{
		ConnectTimeout: time.Second,
		WaitTimeout:    time.Minute,
		Logf:           func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) },
		Reload: func() (string, error) {
			reloads++
			return dsnFor(l.Addr().(*net.TCPAddr).Port), nil
		},
	}
	_, err = Connect(context.Background(), dsnFor(closedPort), o)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != pgerrcode.InvalidPassword {
		t.Fatalf("Connect() error = %v, want the login failure from the reloaded address", err)
	}
	if reloads != 1 {
		t.Errorf("Reload called %d times, want 1", reloads)
	}
	if !strings.Contains(strings.Join(logs, "\n"), "database settings changed") {
		t.Errorf("logs = %q, want a settings change message", logs)
	}
}

var (
	sqlComment = regexp.MustCompile(`--[^\n]*`)
	sqlSpace   = regexp.MustCompile(`\s+`)
	// base matches the tables that releases without the migration know about,
	// optionally schema-qualified or quoted.
	base = `(?:public\.)?"?(?:results|records)"?\b`

	createIndexOnBase    = regexp.MustCompile(`(?i)^CREATE (UNIQUE )?INDEX (CONCURRENTLY )?(IF NOT EXISTS )?\S+ ON (?:ONLY )?` + base)
	createTriggerOnBase  = regexp.MustCompile(`(?i)^CREATE (?:OR REPLACE )?(?:CONSTRAINT )?TRIGGER .* ON ` + base)
	alterBase            = regexp.MustCompile(`(?i)^ALTER TABLE (?:IF EXISTS )?(?:ONLY )?` + base + ` (.*)$`)
	addColumnIfNotExists = regexp.MustCompile(`(?i)^ADD (?:COLUMN )?IF NOT EXISTS \S+ (.*)$`)
	notNull              = regexp.MustCompile(`(?i)\bNOT NULL\b`)
	hasDefault           = regexp.MustCompile(`(?i)\bDEFAULT\b`)
	dropOrRenameBase     = regexp.MustCompile(`(?i)^(?:DROP TABLE (?:IF EXISTS )?` + base + `|ALTER TABLE .*` + base + ` RENAME\b)`)
	referencesBase       = regexp.MustCompile(`(?i)\bREFERENCES ` + base + `(?: ?\([^)]*\))?((?: (?:MATCH \w+|ON (?:UPDATE|DELETE) (?:CASCADE|SET NULL|SET DEFAULT|RESTRICT|NO ACTION)))*)`)
	onDeleteSafe         = regexp.MustCompile(`(?i)ON DELETE (?:CASCADE|SET NULL)`)
	createTable          = regexp.MustCompile(`(?i)^CREATE (?:UNLOGGED )?TABLE (IF NOT EXISTS )?`)
	createIndex          = regexp.MustCompile(`(?i)^CREATE (?:UNIQUE )?INDEX (CONCURRENTLY )?(IF NOT EXISTS )?`)
	addColumn            = regexp.MustCompile(`(?i)^ALTER TABLE .* ADD (?:COLUMN )?(IF NOT EXISTS )?(\w+)`)
	concurrently         = regexp.MustCompile(`(?i)\bCONCURRENTLY\b`)
)

// lintMigration checks an up migration against the additive rules in
// README.md ("Adding a migration"): nothing an earlier
// release's writers or retention agent could violate is added to results or
// records, foreign keys to those tables cascade deletes, every statement is
// idempotent, and CONCURRENTLY statements are alone in their file.
func lintMigration(sql string) []string {
	var problems []string
	bad := func(stmt, format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...)+": "+stmt)
	}
	sql = sqlComment.ReplaceAllString(sql, "")
	var stmts []string
	for _, s := range strings.Split(sql, ";") {
		if s = strings.TrimSpace(sqlSpace.ReplaceAllString(s, " ")); s != "" {
			stmts = append(stmts, s)
		}
	}
	if concurrently.MatchString(sql) && len(stmts) != 1 {
		problems = append(problems, fmt.Sprintf("CONCURRENTLY must be the only statement in its migration, found %d", len(stmts)))
	}
	for _, stmt := range stmts {
		short := stmt
		if len(short) > 80 {
			short = short[:77] + "..."
		}
		if m := createIndexOnBase.FindStringSubmatch(stmt); m != nil {
			if m[1] != "" {
				bad(short, "unique index on a baseline table")
			}
			if m[2] == "" {
				bad(short, "index on a baseline table must be created CONCURRENTLY")
			}
		}
		if createTriggerOnBase.MatchString(stmt) {
			bad(short, "trigger on a baseline table")
		}
		if dropOrRenameBase.MatchString(stmt) {
			bad(short, "baseline table dropped or renamed")
		}
		if m := alterBase.FindStringSubmatch(stmt); m != nil {
			switch col := addColumnIfNotExists.FindStringSubmatch(m[1]); {
			case col == nil:
				bad(short, "only ADD COLUMN IF NOT EXISTS is allowed on a baseline table")
			case notNull.MatchString(col[1]) && !hasDefault.MatchString(col[1]):
				bad(short, "NOT NULL column without DEFAULT on a baseline table")
			}
		}
		for _, m := range referencesBase.FindAllStringSubmatch(stmt, -1) {
			if !onDeleteSafe.MatchString(m[1]) {
				bad(short, "foreign key to a baseline table must be ON DELETE CASCADE or SET NULL")
			}
		}
		if m := createTable.FindStringSubmatch(stmt); m != nil && m[1] == "" {
			bad(short, "CREATE TABLE without IF NOT EXISTS")
		}
		if m := createIndex.FindStringSubmatch(stmt); m != nil && m[2] == "" {
			bad(short, "CREATE INDEX without IF NOT EXISTS")
		}
		if m := addColumn.FindStringSubmatch(stmt); m != nil && m[1] == "" && !strings.EqualFold(m[2], "CONSTRAINT") {
			bad(short, "ADD COLUMN without IF NOT EXISTS")
		}
	}
	return problems
}

// TestLintMigration checks the linter against the rules it must enforce and
// against statements it must accept.
func TestLintMigration(t *testing.T) {
	for _, tc := range []struct {
		sql  string
		want string // substring of a problem, or "" for no problem
	}{
		{"CREATE TABLE IF NOT EXISTS labels (k text NOT NULL)", ""},
		{"CREATE TABLE labels (k text)", "without IF NOT EXISTS"},
		{"CREATE INDEX CONCURRENTLY IF NOT EXISTS results_by_time ON results (created_time)", ""},
		{"CREATE INDEX CONCURRENTLY results_by_time ON results (created_time)", "without IF NOT EXISTS"},
		{"CREATE INDEX IF NOT EXISTS results_by_time ON results (created_time)", "must be created CONCURRENTLY"},
		{"CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS u ON public.records (parent, name)", "unique index on a baseline table"},
		{"CREATE INDEX IF NOT EXISTS l ON labels (k)", ""},
		{"CREATE INDEX CONCURRENTLY IF NOT EXISTS a ON labels (k); CREATE INDEX CONCURRENTLY IF NOT EXISTS b ON labels (v)", "only statement"},
		{"CREATE TRIGGER t AFTER INSERT ON records FOR EACH ROW EXECUTE FUNCTION f()", "trigger on a baseline table"},
		{"ALTER TABLE results ADD COLUMN IF NOT EXISTS metadata jsonb", ""},
		{"ALTER TABLE results ADD COLUMN IF NOT EXISTS kind text NOT NULL DEFAULT ''", ""},
		{"ALTER TABLE results ADD COLUMN IF NOT EXISTS kind text NOT NULL", "NOT NULL column without DEFAULT"},
		{"ALTER TABLE results ADD COLUMN metadata jsonb", "only ADD COLUMN IF NOT EXISTS"},
		{"ALTER TABLE records ADD CONSTRAINT c CHECK (etag <> '')", "only ADD COLUMN IF NOT EXISTS"},
		{"ALTER TABLE records ALTER COLUMN etag SET NOT NULL", "only ADD COLUMN IF NOT EXISTS"},
		{"ALTER TABLE records DROP COLUMN etag", "only ADD COLUMN IF NOT EXISTS"},
		{"ALTER TABLE results RENAME TO results_old", "dropped or renamed"},
		{"DROP TABLE IF EXISTS records", "dropped or renamed"},
		{"CREATE TABLE IF NOT EXISTS labels (parent text, result_id text, FOREIGN KEY (parent, result_id) REFERENCES results (parent, id) ON UPDATE CASCADE ON DELETE CASCADE)", ""},
		{"CREATE TABLE IF NOT EXISTS labels (parent text, result_id text, FOREIGN KEY (parent, result_id) REFERENCES results (parent, id) ON DELETE SET NULL)", ""},
		{"CREATE TABLE IF NOT EXISTS labels (parent text, result_id text, FOREIGN KEY (parent, result_id) REFERENCES results (parent, id))", "ON DELETE CASCADE"},
		{"CREATE TABLE IF NOT EXISTS labels (parent text, result_id text, FOREIGN KEY (parent, result_id) REFERENCES results (parent, id) ON DELETE RESTRICT)", "ON DELETE CASCADE"},
		{"ALTER TABLE labels ADD CONSTRAINT fk FOREIGN KEY (parent, result_id) REFERENCES records (parent, result_id) NOT VALID", "ON DELETE CASCADE"},
		{"ALTER TABLE labels ADD CONSTRAINT fk FOREIGN KEY (k) REFERENCES label_keys (k) NOT VALID", ""},
		{"-- comment mentioning DROP TABLE results\nCREATE TABLE IF NOT EXISTS x (a int)", ""},
	} {
		t.Run(tc.sql, func(t *testing.T) {
			got := lintMigration(tc.sql)
			switch {
			case tc.want == "" && len(got) > 0:
				t.Errorf("unexpected problems: %v", got)
			case tc.want != "" && !strings.Contains(strings.Join(got, "\n"), tc.want):
				t.Errorf("problems %v do not contain %q", got, tc.want)
			}
		})
	}
}

// TestMigrationsAreAdditive lints every embedded up migration after the
// baseline, which creates the tables and is exempt.
func TestMigrationsAreAdditive(t *testing.T) {
	entries, err := files.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		m := fileName.FindStringSubmatch(e.Name())
		if m == nil || m[3] != "up" || m[1] == strings.Repeat("0", len(m[1])-1)+"1" {
			continue
		}
		body, err := files.ReadFile(e.Name())
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range lintMigration(string(body)) {
			t.Errorf("%s: %s", e.Name(), p)
		}
	}
}
