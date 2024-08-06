/*
Copyright 2024 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package retention

import (
	"testing"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/tektoncd/results/pkg/api/server/db"
	"github.com/tektoncd/results/pkg/apis/config"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAgent_job(t *testing.T) {
	// Setup in-memory SQLite database
	dbMem, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to in-memory database: %v", err)
	}

	// Auto migrate the schema
	err = dbMem.AutoMigrate(&db.Record{}, &db.Result{})
	if err != nil {
		t.Fatalf("Failed to migrate database schema: %v", err)
	}

	// Create test agent
	agent := &Agent{
		db:     dbMem,
		Logger: zap.NewExample().Sugar(),

		RetentionPolicy: config.RetentionPolicy{
			MaxRetention: 24 * time.Hour,
		},
		cron: cron.New(),
	}

	// Insert test data
	now := time.Now()
	oldResult := db.Result{UpdatedTime: now.Add(-25 * time.Hour), Parent: "foo", ID: "foo", Name: "foo"}
	newResult := db.Result{UpdatedTime: now, Parent: "foo", ID: "foo-new", Name: "foo-new"}

	oldRecord := db.Record{UpdatedTime: now.Add(-25 * time.Hour), Parent: "foo",
		Result:     oldResult,
		ResultName: "foo", ResultID: "foo", ID: "foo"}
	newRecord := db.Record{UpdatedTime: now, Parent: "foo",
		Result:     newResult,
		ResultName: "foo-new", ResultID: "foo-new", ID: "foo-new"}

	dbMem.Save(&oldRecord)
	dbMem.Create(&newRecord)

	// Run the job
	agent.job()

	// Check if old records and results are deleted
	var count int64
	dbMem.Model(&db.Record{}).Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 record, got %d", count)
	}

	dbMem.Model(&db.Result{}).Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 result, got %d", count)
	}

	// Check if new records and results are retained
	var remainingRecord db.Record
	dbMem.First(&remainingRecord)
	if !remainingRecord.UpdatedTime.Equal(newRecord.UpdatedTime) {
		t.Errorf("Expected remaining record to be the new one")
	}

	var remainingResult db.Result
	dbMem.First(&remainingResult)
	if !remainingResult.UpdatedTime.Equal(newResult.UpdatedTime) {
		t.Errorf("Expected remaining result to be the new one")
	}
}
