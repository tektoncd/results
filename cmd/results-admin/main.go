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

// Package main provides results-admin, which manages the Tekton Results
// database schema.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/pflag"

	"github.com/tektoncd/results/pkg/api/server/config"
	"github.com/tektoncd/results/pkg/api/server/db/migrations"
)

const usage = `Usage: results-admin [flags] <command> [args]

Manages the Tekton Results database schema. Database settings are read from
the same configuration as the API server (DB_HOST, DB_PORT, DB_NAME, DB_USER,
DB_PASSWORD, DB_SSLMODE, DB_SSLROOTCERT).

Commands:
  status                          Print the recorded schema version as JSON.
  check [--structural]            Exit non-zero unless this release can run
                                  against the database. --structural also
                                  compares the baseline tables with their
                                  expected definition.
  migrate-up                      Apply all pending migrations. A database
                                  created by a release without versioned
                                  migrations is adopted first if its tables
                                  match the baseline definition.
  migrate-to <version>            Apply migrations up to <version>.
  force <version> --repaired      Mark a dirty schema clean at <version> after
                                  repairing a failed migration by hand.

Flags:
`

// commandFlags lists the commands and the flags each accepts.
var commandFlags = map[string][]string{
	"status":     nil,
	"check":      {"structural"},
	"migrate-up": nil,
	"migrate-to": nil,
	"force":      {"repaired"},
}

type exitError struct {
	code int
	err  error
}

func (e exitError) Error() string { return e.err.Error() }

func usageError(format string, args ...any) error {
	return exitError{2, fmt.Errorf(format, args...)}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := run(ctx, os.Args[1:], os.Stdout, os.Stderr, func() string { return dsn(config.Get()) })
	if err == nil {
		return
	}
	code := 1
	var e exitError
	if errors.As(err, &e) {
		code = e.code
	}
	_, _ = fmt.Fprintln(os.Stderr, "results-admin:", err)
	stop()
	os.Exit(code) //nolint:gocritic // stop already called

}

// run executes a command. getDSN is called only once the arguments are valid,
// so usage errors do not require database configuration.
func run(ctx context.Context, args []string, stdout, stderr io.Writer, getDSN func() string) error {
	fs := pflag.NewFlagSet("results-admin", pflag.ContinueOnError)
	// Stop at the first non-flag argument (the command name) so that flags
	// meant for the subcommand, parsed separately below, are not mistaken
	// for unknown global flags.
	fs.SetInterspersed(false)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, usage)
		fs.PrintDefaults()
	}
	var o migrations.Options
	fs.DurationVar(&o.ConnectTimeout, "connect-timeout", 10*time.Second, "timeout for establishing a database connection")
	fs.DurationVar(&o.LockTimeout, "lock-timeout", 30*time.Second, "timeout for each lock wait, including the migration lock (0 waits forever)")
	fs.DurationVar(&o.StatementTimeout, "statement-timeout", 0, "timeout for each SQL statement (0 means no timeout)")
	fs.DurationVar(&o.WaitTimeout, "wait-for-database", 0, "keep retrying for this long while the database is not reachable yet, re-reading the configuration before each retry")
	o.Logf = func(format string, args ...any) { _, _ = fmt.Fprintf(stderr, "results-admin: "+format+"\n", args...) }
	// Re-read the mounted configuration while waiting, so that database
	// settings changed after the Job started are picked up.
	o.Reload = func() (string, error) { return getDSN(), nil }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return nil
		}
		return exitError{2, err}
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return usageError("missing command")
	}
	cmd := fs.Arg(0)
	allowed, ok := commandFlags[cmd]
	if !ok {
		return usageError("unknown command %q", cmd)
	}

	sub := pflag.NewFlagSet("results-admin "+cmd, pflag.ContinueOnError)
	sub.SetOutput(stderr)
	structural := sub.Bool("structural", false, "also validate the baseline table structure")
	repaired := sub.Bool("repaired", false, "confirm that the failed migration was completed or undone by hand")
	if err := sub.Parse(fs.Args()[1:]); err != nil {
		return exitError{2, err}
	}
	var err error
	sub.Visit(func(f *pflag.Flag) {
		if !slices.Contains(allowed, f.Name) {
			err = usageError("flag --%s is not valid for %s", f.Name, cmd)
		}
	})
	if err != nil {
		return err
	}
	wantArgs := 0
	if cmd == "migrate-to" || cmd == "force" {
		wantArgs = 1
	}
	if sub.NArg() != wantArgs {
		return usageError("%s takes %d argument(s), got %d", cmd, wantArgs, sub.NArg())
	}

	switch cmd {
	case "status":
		return status(ctx, stdout, getDSN(), o)
	case "check":
		return check(ctx, stdout, getDSN(), o, *structural)
	case "migrate-up":
		s, err := migrations.MigrateUp(ctx, getDSN(), o)
		return report(stdout, cmd, s, err)
	case "migrate-to":
		target, err := strconv.ParseUint(sub.Arg(0), 10, 32)
		if err != nil || target == 0 {
			return usageError("invalid version %q", sub.Arg(0))
		}
		s, err := migrations.MigrateTo(ctx, getDSN(), o, uint(target))
		return report(stdout, cmd, s, err)
	case "force":
		version, err := strconv.ParseUint(sub.Arg(0), 10, 32)
		if err != nil || version == 0 {
			return usageError("invalid version %q", sub.Arg(0))
		}
		if !*repaired {
			return usageError("force requires --repaired: complete or undo the failed migration by hand, then confirm with this flag")
		}
		s, err := migrations.Force(ctx, getDSN(), o, uint(version))
		return report(stdout, cmd, s, err)
	}
	return nil
}

type statusOutput struct {
	migrations.Status
	Effective uint `json:"effective"`
	Latest    uint `json:"latest"`
	Required  uint `json:"required"`
}

func status(ctx context.Context, w io.Writer, dsn string, o migrations.Options) (err error) {
	db, err := migrations.Connect(ctx, dsn, o)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	s, err := migrations.ReadStatus(ctx, db)
	if err != nil {
		return err
	}
	latest, err := migrations.Latest()
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(statusOutput{Status: s, Effective: s.Effective(), Latest: latest, Required: migrations.RequiredSchemaVersion})
}

func check(ctx context.Context, w io.Writer, dsn string, o migrations.Options, structural bool) (err error) {
	db, err := migrations.Connect(ctx, dsn, o)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	s, err := migrations.Check(ctx, db, migrations.RequiredSchemaVersion)
	if err != nil {
		return err
	}
	if structural {
		if err := migrations.ValidateStructure(ctx, db); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(w, "schema version %d (dirty=%t) satisfies required version %d\n", s.Version, s.Dirty, migrations.RequiredSchemaVersion)
	return err
}

func report(w io.Writer, op string, s migrations.Status, err error) error {
	if err != nil {
		if s.Initialized {
			return fmt.Errorf("%w (schema version %d, dirty=%t)", err, s.Version, s.Dirty)
		}
		return err
	}
	_, err = fmt.Fprintf(w, "%s: schema version %d\n", op, s.Version)
	return err
}

// dsn builds a PostgreSQL keyword/value connection string from the Results
// configuration, quoting every value.
func dsn(c *config.Config) string {
	var parts []string
	add := func(k, v string) {
		if v != "" {
			v = strings.ReplaceAll(strings.ReplaceAll(v, `\`, `\\`), `'`, `\'`)
			parts = append(parts, k+"='"+v+"'")
		}
	}
	add("host", c.DB_HOST)
	add("port", c.DB_PORT)
	add("dbname", c.DB_NAME)
	add("user", c.DB_USER)
	add("password", c.DB_PASSWORD)
	add("sslmode", c.DB_SSLMODE)
	add("sslrootcert", c.DB_SSLROOTCERT)
	return strings.Join(parts, " ")
}
