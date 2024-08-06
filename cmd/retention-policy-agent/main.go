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

package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "go.uber.org/automaxprocs"
	"go.uber.org/zap"

	"github.com/tektoncd/results/pkg/api/server/config"
	"github.com/tektoncd/results/pkg/api/server/logger"
	"github.com/tektoncd/results/pkg/retention"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"k8s.io/apimachinery/pkg/util/wait"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	_ "knative.dev/pkg/client/injection/kube/client"
)

func main() {
	serverConfig := config.Get()

	log := logger.Get(serverConfig.LOG_LEVEL)
	// This defer statement will be executed at the end of the application lifecycle, so we do not lose
	// any data in the event of an unhandled error.
	defer log.Sync() //nolint:errcheck

	if serverConfig.DB_USER == "" || serverConfig.DB_PASSWORD == "" {
		log.Fatal("Must provide both DB_USER and DB_PASSWORD") //nolint:gocritic
	}

	// From all available sslmodes, "require", "verify-ca" and "verify-full" require CA cert
	// configured on the client side. We check and fail early if one is not provided.
	if (serverConfig.DB_SSLMODE == "require" || serverConfig.DB_SSLMODE == "verify-ca" || serverConfig.DB_SSLMODE == "verify-full") && serverConfig.DB_SSLROOTCERT == "" {
		log.Fatalf("DB_SSLROOTCERT can't be empty when DB_SSLMODE=%s", serverConfig.DB_SSLMODE)
	}

	// Connect to the database.
	// DSN derived from https://pkg.go.dev/gorm.io/driver/postgres

	var db *gorm.DB
	var err error

	dbURI := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s sslrootcert=%s", serverConfig.DB_HOST, serverConfig.DB_USER, serverConfig.DB_PASSWORD, serverConfig.DB_NAME, serverConfig.DB_PORT, serverConfig.DB_SSLMODE, serverConfig.DB_SSLROOTCERT)
	gormConfig := &gorm.Config{}
	if log.Level() != zap.DebugLevel {
		gormConfig.Logger = gormlogger.Default.LogMode(gormlogger.Silent)
	}
	// Retry database connection, sometimes the database is not ready to accept connection
	err = wait.PollImmediate(10*time.Second, 2*time.Minute, func() (bool, error) { //nolint:staticcheck
		db, err = gorm.Open(postgres.Open(dbURI), gormConfig)
		if err != nil {
			log.Warnf("Error connecting to database (retrying in 10s): %v", err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	var sqlDB *sql.DB

	// Set DB connection limits
	maxIdle := serverConfig.DB_MAX_IDLE_CONNECTIONS
	maxOpen := serverConfig.DB_MAX_OPEN_CONNECTIONS
	if maxOpen > 0 {
		sqlDB, err = db.DB()
		if err != nil {
			log.Fatalf("Error getting database configuration for updating max open connections: %s", err.Error())
		}
		sqlDB.SetMaxOpenConns(maxOpen)
	}
	if maxIdle > 0 {
		sqlDB, err = db.DB()
		if err != nil {
			log.Fatalf("Error getting database configuration for updating max open connections: %s", err.Error())
		}
		sqlDB.SetMaxIdleConns(maxIdle)
	}

	if _, err := retention.NewAgent(db); err != nil {
		log.Fatalf("Failed to start Retention Agent: %v", err)
	}

	select {}
}
