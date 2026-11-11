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

package db_test

import (
	"context"
	"testing"

	"github.com/tektoncd/results/pkg/api/server/db/migrations"
)

// TestMigrations_DeployedSchema verifies that the release's migration Job
// brought the live database to a clean version the deployed API accepts, and
// that the migrated baseline tables match their expected definition.
// Migration behavior itself is covered against a dedicated Postgres database
// by the tests in pkg/api/server/db/migrations.
func TestMigrations_DeployedSchema(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := rawDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	s, err := migrations.Check(ctx, sqlDB, migrations.RequiredSchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	latest, err := migrations.Latest()
	if err != nil {
		t.Fatal(err)
	}
	if s.Dirty || s.Version != int(latest) {
		t.Errorf("schema status = %+v, want clean version %d", s, latest)
	}
	if err := migrations.ValidateStructure(ctx, sqlDB); err != nil {
		t.Error(err)
	}
}

// TestMigrations_MetadataColumns will verify the text[] metadata columns
// and GIN indexes.
func TestMigrations_MetadataColumns(t *testing.T) {
	t.Skip("metadata columns not yet implemented")
}
