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

package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/tektoncd/results/pkg/api/server/config"
)

func TestRunUsageErrors(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, "missing command"},
		{[]string{"nope"}, "unknown command"},
		{[]string{"status", "extra"}, "takes 0 argument(s)"},
		{[]string{"migrate-to"}, "takes 1 argument(s)"},
		{[]string{"migrate-to", "0"}, "invalid version"},
		{[]string{"migrate-to", "x"}, "invalid version"},
		{[]string{"migrate-up", "--structural"}, "not valid for migrate-up"},
		{[]string{"check", "--repaired"}, "not valid for check"},
		{[]string{"adopt-existing"}, "unknown command"},
		{[]string{"force", "1"}, "requires --repaired"},
		{[]string{"force"}, "takes 1 argument(s)"},
		{[]string{"migrate-up", "--repaired"}, "not valid for migrate-up"},
		{[]string{"--bogus", "status"}, "unknown flag: --bogus"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			getDSN := func() string {
				t.Fatal("database configuration read for an invalid invocation")
				return ""
			}
			err := run(context.Background(), tc.args, io.Discard, io.Discard, getDSN)
			var e exitError
			if !errors.As(err, &e) || e.code != 2 || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want usage error containing %q", err, tc.want)
			}
		})
	}
}

// TestForceArgumentOrder guards against a regression where --repaired had to
// precede the version argument. The standard library flag package stops
// parsing flags at the first positional argument, so "force 1 --repaired"
// used to be rejected as "got 2" arguments. pflag parses interspersed flags
// and positionals, so both orders must reach the database connection step
// (and fail there, since no database is configured in this test).
func TestForceArgumentOrder(t *testing.T) {
	for _, args := range [][]string{
		{"force", "1", "--repaired"},
		{"force", "--repaired", "1"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			err := run(context.Background(), args, io.Discard, io.Discard, func() string { return "host=" })
			var e exitError
			if errors.As(err, &e) && e.code == 2 {
				t.Fatalf("run(%v) returned a usage error: %v", args, err)
			}
		})
	}
}

func TestDSN(t *testing.T) {
	c := &config.Config{
		DB_HOST:     "db.example",
		DB_PORT:     "5433",
		DB_NAME:     "tekton results",
		DB_USER:     "admin",
		DB_PASSWORD: `p a's\s=`,
		DB_SSLMODE:  "disable",
	}
	cfg, err := pgx.ParseConfig(dsn(c))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != c.DB_HOST || cfg.Port != 5433 || cfg.Database != c.DB_NAME || cfg.User != c.DB_USER || cfg.Password != c.DB_PASSWORD {
		t.Fatalf("parsed %s@%s:%d/%s password %q", cfg.User, cfg.Host, cfg.Port, cfg.Database, cfg.Password)
	}
}
