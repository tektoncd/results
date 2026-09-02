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

// Package db_test provides end-to-end tests for the Postgres database layer.
//
// Tests exercise the deployed API server (gRPC) in the kind cluster against
// its live Postgres instance, verifying behavior that the SQLite-backed unit
// tests cannot reach:
//   - Postgres-specific error code mapping (pgconn.PgError → gRPC status)
//   - Lister filter/sort/pagination with real Postgres query execution
//   - Schema correctness (column types, indexes, foreign keys, jsonb)
//
// A raw database connection is used only for schema introspection and to
// trigger FK violations that the server's pre-checks would otherwise mask.
//
// Future stories (labels, metadata columns, golang-migrate migrations) add
// their Postgres-specific tests to this package. See the stub files for
// placeholders.
//
// Run:
//
//	go test -v -count=1 -tags=e2e ./test/e2e/db/...
package db_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/tektoncd/results/test/e2e/client"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	// Register the Postgres error space so pgconn.PgError → gRPC code
	// translation is active in the dberrors.Wrap calls used by the FK
	// violation test.
	_ "github.com/tektoncd/results/pkg/api/server/db/errors/postgres"
)

var (
	// adminClient talks to the deployed API server with full CRUD
	// permissions. Used for creating/updating/deleting test data.
	adminClient client.GRPCClient

	// readClient talks to the deployed API server with read-only
	// permissions. Used for list/get assertions where we want to confirm
	// the read path independently.
	readClient client.GRPCClient

	// rawDB is a direct Postgres connection to the live tekton-results
	// database. Used only for schema introspection (information_schema /
	// pg_catalog) and for the FK violation test that must bypass the
	// server's pre-check logic.
	rawDB *gorm.DB
)

func TestMain(m *testing.M) {
	cfg := client.NewEnvConfig()
	var err error

	adminClient, err = client.NewGRPCClientFromConfig(cfg, client.AdminTokenFile, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create admin gRPC client: %v\n", err)
		os.Exit(1)
	}

	readClient, err = client.NewGRPCClientFromConfig(cfg, client.ReadTokenFile, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create read gRPC client: %v\n", err)
		os.Exit(1)
	}

	dsn := buildDSN()
	rawDB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to postgres: %v\n", err)
		os.Exit(1)
	}
	sqlDB, err := rawDB.DB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get underlying sql.DB: %v\n", err)
		os.Exit(1)
	}
	if err := sqlDB.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to ping postgres: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	sqlDB.Close()
	os.Exit(code)
}

func buildDSN() string {
	if url := os.Getenv("DB_URL"); url != "" {
		return url
	}
	host := client.EnvOrDefault("POSTGRES_HOST", "localhost")
	port := client.EnvOrDefault("POSTGRES_PORT", "15432")
	user := client.EnvOrDefault("POSTGRES_USER", "postgres")
	pass := client.EnvOrDefault("POSTGRES_PASSWORD", "")
	dbname := client.EnvOrDefault("POSTGRES_DB", "tekton-results")
	sslmode := client.EnvOrDefault("POSTGRES_SSLMODE", "disable")

	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, pass, dbname, sslmode,
	)
}

func defaultCtx() context.Context {
	return context.Background()
}
