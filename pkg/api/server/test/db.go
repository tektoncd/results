// Copyright 2020 The Tekton Authors
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

package test

import (
	"os"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	// Inject sqlite error checking.
	_ "github.com/tektoncd/results/pkg/api/server/db/errors/sqlite"
)

// NewDB set up a temporary database for testing
func NewDB(t *testing.T) *gorm.DB {
	t.Helper()

	// Create a temporary file
	tmpfile, err := os.CreateTemp("", "testdb")
	if err != nil {
		t.Fatalf("failed to create temp file for db: %v", err)
	}
	t.Log("test database: ", tmpfile.Name())
	t.Cleanup(func() {
		tmpfile.Close()
		os.Remove(tmpfile.Name())
	})

	// Open DB using gorm to use all the nice gorm tools.
	gdb, err := gorm.Open(sqlite.Open(tmpfile.Name()), &gorm.Config{
		// Configure verbose db logging to use testing logger.
		// This will show all SQL statements made if the test fails.
		Logger: logger.New(&testLogger{t: t}, logger.Config{
			LogLevel: logger.Info,
			Colorful: true,
		}),
	})
	if err != nil {
		t.Fatalf("failed to open the results.db: %v", err)
	}

	// Enable foreign key support. Only needed for sqlite instance we use for
	// tests.
	gdb.Exec("PRAGMA foreign_keys = ON;")

	return gdb
}

type testLogger struct {
	t *testing.T
}

func (t *testLogger) Printf(format string, args ...any) {
	t.t.Logf(format, args...)
}
