// Package db defines database models for Result data.
package db

import (
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
	Parent     string `gorm:"primaryKey;index:records_by_name,priority:1"`
	ResultID   string `gorm:"primaryKey"`
	ResultName string `gorm:"index:records_by_name,priority:2"`

	ID   string `gorm:"primaryKey"`
	Name string `gorm:"index:records_by_name,priority:3"`
	Data []byte
}
