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

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/tektoncd/results/pkg/apis/config"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Test_buildCaseStatement(t *testing.T) {
	type args struct {
		policies         []config.Policy
		defaultRetention time.Duration
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "no policies",
			args: args{
				policies:         nil,
				defaultRetention: 30 * 24 * time.Hour,
			},
			want: "NOW() - INTERVAL '2592000.000000 seconds'",
		},
		{
			name: "with policies",
			args: args{
				policies: []config.Policy{
					{
						Selector: config.Selector{
							MatchLabels: map[string][]string{"app": {"foo"}},
						},
						Retention: "10d",
					},
				},
				defaultRetention: 30 * 24 * time.Hour,
			},
			want: "CASE WHEN data->'metadata'->'labels'->>'app' IN ('foo') THEN NOW() - INTERVAL '864000.000000 seconds' ELSE NOW() - INTERVAL '2592000.000000 seconds' END",
		},
		{
			name: "with policies without suffix",
			args: args{
				policies: []config.Policy{
					{
						Selector: config.Selector{
							MatchLabels: map[string][]string{"app": {"foo"}},
						},
						Retention: "10",
					},
				},
				defaultRetention: 30 * 24 * time.Hour,
			},
			want: "CASE WHEN data->'metadata'->'labels'->>'app' IN ('foo') THEN NOW() - INTERVAL '864000.000000 seconds' ELSE NOW() - INTERVAL '2592000.000000 seconds' END",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildCaseStatement(tt.args.policies, tt.args.defaultRetention)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildCaseStatement() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("buildCaseStatement() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_buildWhereClause(t *testing.T) {
	type args struct {
		selector config.Selector
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "empty selector",
			args: args{
				selector: config.Selector{},
			},
			want: "1=1",
		},
		{
			name: "with labels",
			args: args{
				selector: config.Selector{
					MatchLabels: map[string][]string{"app": {"foo"}},
				},
			},
			want: "data->'metadata'->'labels'->>'app' IN ('foo')",
		},
		{
			name: "with annotations",
			args: args{
				selector: config.Selector{
					MatchAnnotations: map[string][]string{"tekton.dev/image": {"bar"}},
				},
			},
			want: "data->'metadata'->'annotations'->>'tekton.dev/image' IN ('bar')",
		},
		{
			name: "with status",
			args: args{
				selector: config.Selector{
					MatchStatuses: []string{"Succeeded"},
				},
			},
			want: "data->'status'->'conditions'->0->>'reason' IN ('Succeeded')",
		},
		{
			name: "with namespace",
			args: args{
				selector: config.Selector{
					MatchNamespaces: []string{"prod"},
				},
			},
			want: "parent IN ('prod')",
		},
		{
			name: "with multiple conditions",
			args: args{
				selector: config.Selector{
					MatchNamespaces: []string{"prod"},
					MatchLabels:     map[string][]string{"app": {"foo"}},
					MatchStatuses:   []string{"Succeeded"},
				},
			},
			want: "parent IN ('prod') AND data->'metadata'->'labels'->>'app' IN ('foo') AND data->'status'->'conditions'->0->>'reason' IN ('Succeeded')",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildWhereClause(tt.args.selector)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildWhereClause() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("buildWhereClause() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAgent_cleanupResults(t *testing.T) {
	// Create a new mock database
	mockDb, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("unexpected error when opening a stub database connection: %v", err)
	}
	defer mockDb.Close()

	// Create a new gorm DB using the sqlmock connection
	gormDb, err := gorm.Open(postgres.New(postgres.Config{
		Conn: mockDb,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("unexpected error when opening a gorm database: %v", err)
	}

	// Use a no-op logger for tests
	logger := zap.NewNop()

	agent := &Agent{
		db:     gormDb,
		Logger: logger.Sugar(),
	}

	tests := []struct {
		name       string
		recordType string
		// regex that must match the Exec'd SQL
		wantQuery string
	}{
		{
			name:       "cleanup pipelineruns",
			recordType: "tekton.dev/v1.PipelineRun",
			// match core structure; dot matches newline; allow flexible whitespace
			wantQuery: `(?s)DELETE\s+FROM\s+results\s+WHERE\s+id\s+IN\s*\(\s*SELECT\s+result_id\s+FROM\s*\(\s*SELECT\s+r\.result_id,.*r\.updated_time,.*AS\s+expiration_time.*FROM\s+records\s+r.*WHERE\s+r\.type\s*=\s*'tekton\.dev/v1\.PipelineRun'.*\)\s+AS\s+subquery\s+WHERE\s+updated_time\s*<\s*expiration_time\s*\)`,
		},
		{
			name:       "cleanup taskruns",
			recordType: "tekton.dev/v1.TaskRun",
			// allow an optional AND NOT EXISTS (...) inside the inner WHERE
			wantQuery: `(?s)DELETE\s+FROM\s+results\s+WHERE\s+id\s+IN\s*\(\s*SELECT\s+result_id\s+FROM\s*\(\s*SELECT\s+r\.result_id,.*r\.updated_time,.*AS\s+expiration_time.*FROM\s+records\s+r.*WHERE\s+r\.type\s*=\s*'tekton\.dev/v1\.TaskRun'\s*(?:AND\s+NOT\s+EXISTS\s*\(.*\)\s*)?.*\)\s+AS\s+subquery\s+WHERE\s+updated_time\s*<\s*expiration_time\s*\)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Expect the Exec of the SQL that matches the regex. Return success.
			mock.ExpectExec(tt.wantQuery).WillReturnResult(sqlmock.NewResult(0, 1))

			// Call function. It logs error internally; it doesn't return error.
			agent.cleanupResults("NOW()", tt.recordType)

			// Verify expectations.
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unfulfilled expectations: %v", err)
			}
		})
	}
}
