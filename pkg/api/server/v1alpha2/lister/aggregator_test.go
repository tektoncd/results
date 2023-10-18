// Copyright 2021 The Tekton Authors
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

package lister

import (
	"testing"
)

func TestCheckAndBuildGroupQuery(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		want    string
		wantErr bool
	}{
		{
			name:    "valid time query with one part",
			query:   "minute",
			want:    "EXTRACT(EPOCH FROM DATE_TRUNC('minute', (data->'metadata'->>'creationTimestamp')::TIMESTAMP WITH TIME ZONE)) AS group_value",
			wantErr: false,
		},
		{
			name:    "valid time query with two parts and startTime",
			query:   "hour startTime",
			want:    "EXTRACT(EPOCH FROM DATE_TRUNC('hour', (data->'status'->>'startTime')::TIMESTAMP WITH TIME ZONE)) AS group_value",
			wantErr: false,
		},
		{
			name:    "valid time query with two parts and completionTime",
			query:   "minute completionTime",
			want:    "EXTRACT(EPOCH FROM DATE_TRUNC('minute', (data->'status'->>'completionTime')::TIMESTAMP WITH TIME ZONE)) AS group_value",
			wantErr: false,
		},
		{
			name:    "valid non-time query with parent",
			query:   "namespace",
			want:    "data->'metadata'->>'namespace' AS group_value",
			wantErr: false,
		},
		{
			name:    "valid non-time query with pipeline",
			query:   "pipeline",
			want:    "CONCAT(data->'metadata'->>'namespace', '/', data->'metadata'->'labels'->>'tekton.dev/pipeline') AS group_value",
			wantErr: false,
		},
		{
			name:    "valid non-time query with repository",
			query:   "repository",
			want:    "CONCAT(data->'metadata'->>'namespace', '/', data->'metadata'->'annotations'->>'pipelinesascode.tekton.dev/repository') AS group_value",
			wantErr: false,
		},
		{
			name:    "invalid query",
			query:   "invalid",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := checkAndBuildGroupQuery(tt.query)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkAndBuildGroupQuery() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("checkAndBuildGroupQuery() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckAndBuildOrderBy(t *testing.T) {
	allowedFields := []string{"total", "max_duration", "min_duration", "avg_duration", "succeeded"}

	tests := []struct {
		name    string
		query   string
		want    string
		wantErr bool
	}{
		{
			name:    "valid query with ascending order",
			query:   "total ASC",
			want:    "total ASC",
			wantErr: false,
		},
		{
			name:    "valid query with descending order",
			query:   "max_duration DESC",
			want:    "max_duration DESC",
			wantErr: false,
		},
		{
			name:    "invalid query with no order",
			query:   "total",
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid query with wrong field",
			query:   "wrongField ASC",
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid query with wrong order",
			query:   "total WRONG",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := checkAndBuildOrderBy(tt.query, allowedFields)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkAndBuildOrderBy() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("checkAndBuildOrderBy() = %v, want %v", got, tt.want)
			}
		})
	}
}
