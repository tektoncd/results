// Copyright 2025 The Tekton Authors
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

// Package db defines database models for Result data.
package db

import (
	"errors"

	gormlogger "gorm.io/gorm/logger"
)

var (
	logLevel = map[string]gormlogger.LogLevel{
		"silent": gormlogger.Silent,
		"error":  gormlogger.Error,
		"warn":   gormlogger.Warn,
		"info":   gormlogger.Info,
	}
)

// SetLogLevel for the Default GormLogger
func SetLogLevel(level string) error {
	if _, ok := logLevel[level]; !ok {
		return errors.New("undefined log level for sql")
	}
	gormlogger.Default.LogMode(logLevel[level])
	return nil
}
