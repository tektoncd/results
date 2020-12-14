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
	Parent      string `gorm:"primaryKey;index:results_by_name,priority:1"`
	ID          string `gorm:"primaryKey"`
	Name        string `gorm:"index:results_by_name,priority:2"`
	CreatedTime time.Time
	UpdatedTime time.Time
	Annotations dbjson
}

func (r Result) String() string {
	return fmt.Sprintf("(%s, %s)", r.Parent, r.ID)
}

// Record is the database model of a Record
type Record struct {
	Parent     string `gorm:"primaryKey;index:records_by_name,priority:1"`
	ResultID   string `gorm:"primaryKey"`
	ID         string `gorm:"primaryKey"`
	ResultName string `gorm:"index:records_by_name,priority:2"`
	Name       string `gorm:"index:records_by_name,priority:3"`
	Data       []byte
}

type dbjson json.RawMessage

// Scan scan value into Jsonb, implements sql.Scanner interface
func (j *dbjson) Scan(value interface{}) error {
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New(fmt.Sprint("Failed to unmarshal JSONB value:", value))
	}

	result := json.RawMessage{}
	err := json.Unmarshal(bytes, &result)
	*j = dbjson(result)
	return err
}

// Value return json value, implement driver.Valuer interface
func (j dbjson) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	return json.RawMessage(j).MarshalJSON()
}
