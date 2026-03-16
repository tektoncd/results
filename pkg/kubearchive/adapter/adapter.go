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

package adapter

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kubearchive/kubearchive/pkg/database/interfaces"
	"github.com/kubearchive/kubearchive/pkg/models"
	"github.com/tektoncd/results/pkg/api/server/db"
	"gorm.io/gorm"
)

// TektonResultsAdapter implements the Kubearchive DBReader interface
// It queries Tekton Results database and transforms data to Kubearchive format
type TektonResultsAdapter struct {
	db          *gorm.DB
	transformer *ResourceTransformer
}

// NewTektonResultsAdapter creates a new adapter instance
func NewTektonResultsAdapter(database *gorm.DB) *TektonResultsAdapter {
	return &TektonResultsAdapter{
		db:          database,
		transformer: &ResourceTransformer{},
	}
}

// QueryResources queries resources with filters and returns them in Kubearchive format
func (a *TektonResultsAdapter) QueryResources(
	ctx context.Context,
	kind, apiVersion, namespace, name, continueId, continueDate string,
	labelFilters *models.LabelFilters,
	creationTimestampAfter, creationTimestampBefore *time.Time,
	limit int,
) ([]models.Resource, error) {
	// Build GORM query
	query := a.db.WithContext(ctx).Model(&db.Record{})

	// Construct expected Type string: "{apiVersion}.{kind}"
	expectedType := fmt.Sprintf("%s.%s", apiVersion, kind)
	query = query.Where("type = ?", expectedType)

	// Filter by namespace (Parent field)
	if namespace != "" {
		query = query.Where("parent = ?", namespace)
	}

	// Filter by name with wildcard support
	// Query the JSONB data.metadata.name field, not the Record.Name column
	if name != "" {
		if strings.Contains(name, "*") {
			// Convert wildcard to SQL LIKE pattern
			pattern := strings.ReplaceAll(name, "*", "%")
			query = query.Where("data->'metadata'->>'name' LIKE ?", pattern)
		} else {
			query = query.Where("data->'metadata'->>'name' = ?", name)
		}
	}

	// Apply timestamp filters
	if creationTimestampAfter != nil {
		query = query.Where("created_time > ?", *creationTimestampAfter)
	}
	if creationTimestampBefore != nil {
		query = query.Where("created_time < ?", *creationTimestampBefore)
	}

	// Apply label filters if provided
	if labelFilters != nil {
		query = a.applyLabelFilters(query, labelFilters)
	}

	// Implement cursor-based pagination
	if continueDate != "" && continueId != "" {
		// Parse continue date
		continueTime, err := time.Parse(time.RFC3339, continueDate)
		if err != nil {
			return nil, fmt.Errorf("invalid continue date: %w", err)
		}

		// Pagination: created_time < continueTime OR (created_time = continueTime AND id < continueId)
		query = query.Where(
			"created_time < ? OR (created_time = ? AND id < ?)",
			continueTime, continueTime, continueId,
		)
	}

	// Order by created_time DESC, id DESC for consistent pagination
	query = query.Order("created_time DESC, id DESC")

	// Limit results
	if limit > 0 {
		query = query.Limit(limit)
	}

	// Execute query
	var records []db.Record
	if err := query.Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to query records: %w", err)
	}

	// Transform records to Kubearchive Resource format
	return a.transformRecords(records)
}

// StreamResources streams resources matching the filters, calling fn for each resource
// This is more memory efficient than QueryResources for large result sets
func (a *TektonResultsAdapter) StreamResources(
	ctx context.Context,
	kind, apiVersion, namespace, name, continueId, continueDate string,
	labelFilters *models.LabelFilters,
	creationTimestampAfter, creationTimestampBefore *time.Time,
	limit int,
	fn func(resource models.Resource) error,
) error {
	// Build GORM query (same as QueryResources)
	query := a.db.WithContext(ctx).Model(&db.Record{})

	// Construct expected Type string
	expectedType := fmt.Sprintf("%s.%s", apiVersion, kind)
	query = query.Where("type = ?", expectedType)

	// Apply filters (same logic as QueryResources)
	if namespace != "" {
		query = query.Where("parent = ?", namespace)
	}

	// Filter by name with wildcard support
	// Query the JSONB data.metadata.name field, not the Record.Name column
	if name != "" {
		if strings.Contains(name, "*") {
			pattern := strings.ReplaceAll(name, "*", "%")
			query = query.Where("data->'metadata'->>'name' LIKE ?", pattern)
		} else {
			query = query.Where("data->'metadata'->>'name' = ?", name)
		}
	}

	if creationTimestampAfter != nil {
		query = query.Where("created_time > ?", *creationTimestampAfter)
	}
	if creationTimestampBefore != nil {
		query = query.Where("created_time < ?", *creationTimestampBefore)
	}

	if labelFilters != nil {
		query = a.applyLabelFilters(query, labelFilters)
	}

	if continueDate != "" && continueId != "" {
		continueTime, err := time.Parse(time.RFC3339, continueDate)
		if err != nil {
			return fmt.Errorf("invalid continue date: %w", err)
		}
		query = query.Where(
			"created_time < ? OR (created_time = ? AND id < ?)",
			continueTime, continueTime, continueId,
		)
	}

	query = query.Order("created_time DESC, id DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	// Use FindInBatches to stream results
	batchSize := 100
	return query.FindInBatches(&[]db.Record{}, batchSize, func(tx *gorm.DB, batch int) error {
		var records []db.Record
		if err := tx.Find(&records).Error; err != nil {
			return fmt.Errorf("failed to fetch batch: %w", err)
		}

		// Transform and call fn for each record
		for _, record := range records {
			k8sJSON, err := a.transformer.RecordToK8sResource(&record)
			if err != nil {
				return fmt.Errorf("failed to transform record %s: %w", record.ID, err)
			}

			resource := models.Resource{
				Uuid: record.ID,
				Data: k8sJSON,
				Date: record.CreatedTime.UTC().Format(time.RFC3339),
				Id:   dateToSequentialID(record.CreatedTime),
			}

			if err := fn(resource); err != nil {
				return err
			}
		}

		return nil
	}).Error
}

// QueryResourceByUID queries a single resource by UID
func (a *TektonResultsAdapter) QueryResourceByUID(
	ctx context.Context,
	kind, apiVersion, namespace, uid string,
) (*models.Resource, error) {
	// Query by ID field (which contains the UUID)
	var record db.Record
	query := a.db.WithContext(ctx).Model(&db.Record{}).Where("id = ?", uid)

	// Also filter by type for safety
	expectedType := fmt.Sprintf("%s.%s", apiVersion, kind)
	query = query.Where("type = ?", expectedType)

	// Filter by namespace if provided
	if namespace != "" {
		query = query.Where("parent = ?", namespace)
	}

	if err := query.First(&record).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // Not found, return nil without error
		}
		return nil, fmt.Errorf("failed to query record by UID: %w", err)
	}

	// Transform single record
	resources, err := a.transformRecords([]db.Record{record})
	if err != nil {
		return nil, err
	}

	if len(resources) == 0 {
		return nil, nil
	}

	return &resources[0], nil
}

// QueryLogURLByName returns log URL for a resource by name
// Not implemented for Tekton Results
func (a *TektonResultsAdapter) QueryLogURLByName(
	ctx context.Context,
	kind, apiVersion, namespace, name, containerName string,
) (*interfaces.LogRecord, error) {
	return nil, fmt.Errorf("log URL query by name is not implemented")
}

// QueryLogURLByUID returns log URL for a resource by UID
// Not implemented for Tekton Results
func (a *TektonResultsAdapter) QueryLogURLByUID(
	ctx context.Context,
	kind, apiVersion, namespace, uid, containerName string,
) (*interfaces.LogRecord, error) {
	return nil, fmt.Errorf("log URL query by UID is not implemented")
}

// Ping checks database connectivity
func (a *TektonResultsAdapter) Ping(ctx context.Context) error {
	sqlDB, err := a.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying SQL DB: %w", err)
	}
	return sqlDB.PingContext(ctx)
}

// QueryDatabaseSchemaVersion returns the Kubearchive schema version
// For Tekton Results integration, we return the current Kubearchive PostgreSQL schema version
func (a *TektonResultsAdapter) QueryDatabaseSchemaVersion(ctx context.Context) (string, error) {
	// Return Kubearchive's current max schema version for PostgreSQL
	return "6", nil
}

// CloseDB closes the database connection
func (a *TektonResultsAdapter) CloseDB() error {
	sqlDB, err := a.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying SQL DB: %w", err)
	}
	return sqlDB.Close()
}

// Init initializes the database (no-op since DB is already initialized)
func (a *TektonResultsAdapter) Init(env map[string]string) error {
	// No-op: database is already initialized by Tekton Results
	return nil
}

// transformRecords converts Tekton Results Records to Kubearchive Resources
func (a *TektonResultsAdapter) transformRecords(records []db.Record) ([]models.Resource, error) {
	resources := make([]models.Resource, 0, len(records))

	for _, record := range records {
		// Transform Record.Data to Kubernetes resource JSON
		k8sJSON, err := a.transformer.RecordToK8sResource(&record)
		if err != nil {
			return nil, fmt.Errorf("failed to transform record %s: %w", record.ID, err)
		}

		// Create Kubearchive Resource
		resource := models.Resource{
			Uuid: record.ID,
			Data: k8sJSON,
			Date: record.CreatedTime.UTC().Format(time.RFC3339),
			Id:   dateToSequentialID(record.CreatedTime),
		}

		resources = append(resources, resource)
	}

	return resources, nil
}

// applyLabelFilters applies label selector filters to the GORM query
// Uses PostgreSQL JSONB operators to query labels in Record.Data
func (a *TektonResultsAdapter) applyLabelFilters(
	query *gorm.DB,
	labelFilters *models.LabelFilters,
) *gorm.DB {
	// Label exists: data->'metadata'->'labels' ? 'key'
	if labelFilters.Exists != nil {
		for key := range labelFilters.Exists {
			query = query.Where("data->'metadata'->'labels' ? ?", key)
		}
	}

	// Label equals: data->'metadata'->'labels'->>'key' = 'value'
	if labelFilters.Equals != nil {
		for key, value := range labelFilters.Equals {
			query = query.Where("data->'metadata'->'labels'->>? = ?", key, value)
		}
	}

	// Label not exists: NOT (data->'metadata'->'labels' ? 'key')
	if labelFilters.NotExists != nil {
		for key := range labelFilters.NotExists {
			query = query.Where("NOT (data->'metadata'->'labels' ? ?)", key)
		}
	}

	// Label not equals
	if labelFilters.NotEquals != nil {
		for key, value := range labelFilters.NotEquals {
			query = query.Where(
				"(NOT (data->'metadata'->'labels' ? ?) OR data->'metadata'->'labels'->>? != ?)",
				key, key, value,
			)
		}
	}

	// Label in set
	if labelFilters.In != nil {
		for key, values := range labelFilters.In {
			query = query.Where("data->'metadata'->'labels'->>? IN ?", key, values)
		}
	}

	// Label not in set
	if labelFilters.NotIn != nil {
		for key, values := range labelFilters.NotIn {
			query = query.Where(
				"(NOT (data->'metadata'->'labels' ? ?) OR data->'metadata'->'labels'->>? NOT IN ?)",
				key, key, values,
			)
		}
	}

	return query
}

// dateToSequentialID converts a timestamp to a sequential ID for pagination
// Uses Unix nanoseconds to ensure uniqueness and ordering
func dateToSequentialID(t time.Time) int64 {
	return t.UnixNano()
}
