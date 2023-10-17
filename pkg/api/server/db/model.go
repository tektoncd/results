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

// Package db defines database models for Result data.
package db

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Result is the database model of a Result.
type Result struct {
	Parent      string      `gorm:"primaryKey;uniqueIndex:results_by_name,priority:1;size:64;"`
	ID          string      `gorm:"primaryKey;size:64;"`
	Name        string      `gorm:"uniqueIndex:results_by_name,priority:2;size:64;"`
	Annotations Annotations `gorm:"type:jsonb;"`

	CreatedTime time.Time `gorm:"default:current_timestamp;"`
	UpdatedTime time.Time `gorm:"default:current_timestamp;"`

	Summary RecordSummary `gorm:"embedded;embeddedPrefix:recordsummary_;"`

	Etag string `gorm:"size:128;"`
}

// RecordSummary is the database model of a Result.RecordSummary.
type RecordSummary struct {
	Record string `gorm:"size:256;"`
	// Napkin Math (with a bit of buffer): 256 (DNS Subdomain) * 3 (Group +
	// Version + Kind).
	Type        string `gorm:"size:768;"`
	StartTime   *time.Time
	EndTime     *time.Time
	Status      int32
	Annotations Annotations `gorm:"type:jsonb;"`
}

func (r Result) String() string {
	return fmt.Sprintf("(%s, %s)", r.Parent, r.ID)
}

// Record is the database model of a Record
type Record struct {
	// Result is used to create the relationship between the Result and Records
	// table. Data will not be returned here during reads. Use the foreign key
	// fields instead.
	Result     Result `gorm:"foreignKey:Parent,ResultID;references:Parent,ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Parent     string `gorm:"primaryKey;uniqueIndex:records_by_name,priority:1;size:64;"`
	ResultID   string `gorm:"primaryKey;size:64;"`
	ResultName string `gorm:"uniqueIndex:records_by_name,priority:2;size:64;"`

	ID   string `gorm:"primaryKey;size:64;"`
	Name string `gorm:"index:records_by_name,priority:3;size:64;"`

	// Napkin Math (with a bit of buffer): 256 (DNS Subdomain) * 3 (Group +
	// Version + Kind).
	Type string `gorm:"size:768;"`
	Data []byte `gorm:"type:jsonb;"`

	CreatedTime time.Time `gorm:"default:current_timestamp;"`
	UpdatedTime time.Time `gorm:"default:current_timestamp;"`

	Etag string `gorm:"size:128;"`
}

// Annotations is a custom-defined type of a gorm model field.
type Annotations map[string]string

// Scan resolves serialized data read from database into an Annotation.
// This implements the sql.Scanner interface.
func (ann *Annotations) Scan(value any) error {
	if ann == nil {
		return errors.New("the annotation pointer mustn't be nil")
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("wanted []byte, got %T: %+v", value, value)
	}
	return json.Unmarshal(bytes, ann)
}

// Value returns the value of Annotations for database driver. This implements driver.Valuer.
// gorm uses this function to convert a database model's Annotation field into a type that gorm
// driver can write into the database.
func (ann Annotations) Value() (driver.Value, error) {
	bytes, err := json.Marshal(ann)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}
