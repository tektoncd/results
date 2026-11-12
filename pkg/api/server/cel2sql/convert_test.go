// Copyright 2023 The Tekton Authors
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

package cel2sql

import (
	"errors"
	"testing"

	"github.com/tektoncd/results/pkg/api/server/cel"

	"github.com/google/go-cmp/cmp"
)

func TestConvertRecordExpressions(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantSQL  string
		wantArgs []any
	}{
		{
			name:     "simple expression",
			in:       `name == "foo"`,
			wantSQL:  "(name = ?)",
			wantArgs: []any{"foo"},
		},
		{
			name:     "select expression",
			in:       `data.metadata.namespace == "default"`,
			wantSQL:  "((data->'metadata'->>'namespace') = ?)",
			wantArgs: []any{"default"},
		},
		{
			name:     "type coercion with a dyn expression in the left hand side",
			in:       `data.status.completionTime > timestamp("2022/10/30T21:45:00.000Z")`,
			wantSQL:  "((data->'status'->>'completionTime')::TIMESTAMP WITH TIME ZONE > ?::TIMESTAMP WITH TIME ZONE)",
			wantArgs: []any{"2022/10/30T21:45:00.000Z"},
		},
		{
			name:     "type coercion with a dyn expression in the right hand side",
			in:       `timestamp("2022/10/30T21:45:00.000Z") < data.status.completionTime`,
			wantSQL:  "(?::TIMESTAMP WITH TIME ZONE < (data->'status'->>'completionTime')::TIMESTAMP WITH TIME ZONE)",
			wantArgs: []any{"2022/10/30T21:45:00.000Z"},
		},
		{
			name:     "in operator",
			in:       `data.metadata.namespace in ["foo", "bar"]`,
			wantSQL:  "((data->'metadata'->>'namespace') IN (?, ?))",
			wantArgs: []any{"foo", "bar"},
		},
		{
			name:     "index operator",
			in:       `data.metadata.labels["foo"] == "bar"`,
			wantSQL:  "((data->'metadata'->'labels'->>?) = ?)",
			wantArgs: []any{"foo", "bar"},
		},
		{
			name:     "concatenate strings",
			in:       `name + "bar" == "foobar"`,
			wantSQL:  "(CONCAT(name, ?) = ?)",
			wantArgs: []any{"bar", "foobar"},
		},
		{
			name:     "multiple concatenate strings",
			in:       `name + "bar" + "baz" == "foobarbaz"`,
			wantSQL:  "(CONCAT(name, ?, ?) = ?)",
			wantArgs: []any{"bar", "baz", "foobarbaz"},
		},
		{
			name:     "contains string function",
			in:       `data.metadata.name.contains("foo")`,
			wantSQL:  "POSITION(? IN (data->'metadata'->>'name')) <> 0",
			wantArgs: []any{"foo"},
		},
		{
			name:     "not contains string function",
			in:       `!(data.metadata.annotations.contains("foo"))`,
			wantSQL:  "NOT (POSITION(? IN (data->'metadata'->>'annotations')) <> 0)",
			wantArgs: []any{"foo"},
		},
		{
			name:     "complex not expressions",
			in:       `!(data.metadata.annotations.contains("foo")) && data.metadata.name.endsWith("bar")`,
			wantSQL:  "(NOT (POSITION(? IN (data->'metadata'->>'annotations')) <> 0) AND (data->'metadata'->>'name') LIKE '%' || ?)",
			wantArgs: []any{"foo", "bar"},
		},
		{
			name:     "endsWith string function",
			in:       `data.metadata.name.endsWith("bar")`,
			wantSQL:  "(data->'metadata'->>'name') LIKE '%' || ?",
			wantArgs: []any{"bar"},
		},
		{
			name:     "getDate function",
			in:       `data.status.completionTime.getDate() == 2`,
			wantSQL:  "(EXTRACT(DAY FROM (data->'status'->>'completionTime')::TIMESTAMP WITH TIME ZONE) = 2)",
			wantArgs: nil,
		},
		{
			name:     "getDayOfMonth function",
			in:       `data.status.completionTime.getDayOfMonth() == 2`,
			wantSQL:  "((EXTRACT(DAY FROM (data->'status'->>'completionTime')::TIMESTAMP WITH TIME ZONE) - 1) = 2)",
			wantArgs: nil,
		},
		{
			name:     "getDayOfWeek function",
			in:       `data.status.completionTime.getDayOfWeek() > 0`,
			wantSQL:  "(EXTRACT(DOW FROM (data->'status'->>'completionTime')::TIMESTAMP WITH TIME ZONE) > 0)",
			wantArgs: nil,
		},
		{
			name:     "getDayOfYear function",
			in:       `data.status.completionTime.getDayOfYear() > 15`,
			wantSQL:  "((EXTRACT(DOY FROM (data->'status'->>'completionTime')::TIMESTAMP WITH TIME ZONE) - 1) > 15)",
			wantArgs: nil,
		},
		{
			name:     "getFullYear function",
			in:       `data.status.completionTime.getFullYear() >= 2022`,
			wantSQL:  "(EXTRACT(YEAR FROM (data->'status'->>'completionTime')::TIMESTAMP WITH TIME ZONE) >= 2022)",
			wantArgs: nil,
		},
		{
			name:     "matches function",
			in:       `data.metadata.name.matches("^foo.*$")`,
			wantSQL:  "(data->'metadata'->>'name') ~ ?",
			wantArgs: []any{"^foo.*$"},
		},
		{
			name:     "startsWith string function",
			in:       `data.metadata.name.startsWith("bar")`,
			wantSQL:  "(data->'metadata'->>'name') LIKE ? || '%'",
			wantArgs: []any{"bar"},
		},
		{
			name:     "data_type field",
			in:       `data_type == PIPELINE_RUN`,
			wantSQL:  "(type = ?)",
			wantArgs: []any{"tekton.dev/v1.PipelineRun"},
		},
		{
			name:     "index operator with numeric argument in JSON arrays",
			in:       `data_type == "tekton.dev/v1.TaskRun" && data.status.conditions[0].status == "True"`,
			wantSQL:  "((type = ?) AND ((data->'status'->'conditions'->0->>'status') = ?))",
			wantArgs: []any{"tekton.dev/v1.TaskRun", "True"},
		},
		{
			name:     "index operator as first operation in JSON object",
			in:       `data_type == "tekton.dev/v1.TaskRun" && data["status"].conditions[0].status == "True"`,
			wantSQL:  "((type = ?) AND ((data->?->'conditions'->0->>'status') = ?))",
			wantArgs: []any{"tekton.dev/v1.TaskRun", "status", "True"},
		},
		{
			name:     "index operator with string argument in JSON object",
			in:       `data_type == "tekton.dev/v1.TaskRun" && data.status["conditions"][0].status == "True"`,
			wantSQL:  "((type = ?) AND ((data->'status'->?->0->>'status') = ?))",
			wantArgs: []any{"tekton.dev/v1.TaskRun", "conditions", "True"},
		},
		{
			name:     "complex expression with subgroups",
			in:       `data.metadata.annotations["foo"] == "bar" && !(data.metadata.name.endsWith("baz") || (data.metadata.name.startsWith("foo")))`,
			wantSQL:  `(((data->'metadata'->'annotations'->>?) = ?) AND NOT (((data->'metadata'->>'name') LIKE '%' || ? OR (data->'metadata'->>'name') LIKE ? || '%')))`,
			wantArgs: []any{"foo", "bar", "baz", "foo"},
		},
		{
			name:     "complex expression involving multiple subgroups and operators",
			in:       `data.metadata.annotations["foo"] == "bar" && !(data.metadata.name.endsWith("baz")) || (data.metadata.name.startsWith("foo") && data.metadata.name.contains("bar"))`,
			wantSQL:  `((((data->'metadata'->'annotations'->>?) = ?) AND NOT ((data->'metadata'->>'name') LIKE '%' || ?)) OR ((data->'metadata'->>'name') LIKE ? || '%' AND POSITION(? IN (data->'metadata'->>'name')) <> 0))`,
			wantArgs: []any{"foo", "bar", "baz", "foo", "bar"},
		},
		// Injection regression tests
		{
			name:     "SQL injection via string value is blocked",
			in:       `name == "x') OR 1=1 OR ('a'='b"`,
			wantSQL:  "(name = ?)",
			wantArgs: []any{"x') OR 1=1 OR ('a'='b"},
		},
		{
			name:     "SQL injection via contains argument is blocked",
			in:       `data.metadata.name.contains("') OR 1=1 --")`,
			wantSQL:  "POSITION(? IN (data->'metadata'->>'name')) <> 0",
			wantArgs: []any{"') OR 1=1 --"},
		},
		{
			name:     "single-quote in JSON accessor key is parameterized",
			in:       `data.metadata.labels["it''s"] == "safe"`,
			wantSQL:  "((data->'metadata'->'labels'->>?) = ?)",
			wantArgs: []any{"it''s", "safe"},
		},
	}

	env, err := cel.NewRecordsEnv()
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotSQL, gotArgs, err := Convert(env, test.in)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("wantSQL: %+v\n", test.wantSQL)
			t.Logf("gotSQL:  %+v\n", gotSQL)
			if diff := cmp.Diff(test.wantSQL, gotSQL); diff != "" {
				t.Errorf("SQL mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.wantArgs, gotArgs); diff != "" {
				t.Errorf("args mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestConvertResultExpressions(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantSQL  string
		wantArgs []any
	}{
		{
			name:     "Result.Parent field",
			in:       `parent.endsWith("bar")`,
			wantSQL:  "parent LIKE '%' || ?",
			wantArgs: []any{"bar"},
		},
		{
			name:     "Result.Uid field",
			in:       `uid == "foo"`,
			wantSQL:  "(id = ?)",
			wantArgs: []any{"foo"},
		},
		{
			name:     "Result.Annotations field",
			in:       `annotations["repo"] == "tektoncd/results"`,
			wantSQL:  `annotations @> ?::jsonb`,
			wantArgs: []any{`{"repo":"tektoncd/results"}`},
		},
		{
			name:     "Result.Annotations field reversed operands",
			in:       `"tektoncd/results" == annotations["repo"]`,
			wantSQL:  `annotations @> ?::jsonb`,
			wantArgs: []any{`{"repo":"tektoncd/results"}`},
		},
		{
			name:     "other operators involving the Result.Annotations field",
			in:       `annotations["repo"].startsWith("tektoncd")`,
			wantSQL:  "annotations->>? LIKE ? || '%'",
			wantArgs: []any{"repo", "tektoncd"},
		},
		{
			name:     "Result.CreateTime field",
			in:       `create_time > timestamp("2022/10/30T21:45:00.000Z")`,
			wantSQL:  "(created_time > ?::TIMESTAMP WITH TIME ZONE)",
			wantArgs: []any{"2022/10/30T21:45:00.000Z"},
		},
		{
			name:     "Result.UpdateTime field",
			in:       `update_time > timestamp("2022/10/30T21:45:00.000Z")`,
			wantSQL:  "(updated_time > ?::TIMESTAMP WITH TIME ZONE)",
			wantArgs: []any{"2022/10/30T21:45:00.000Z"},
		},
		{
			name:     "Result.Summary.Record field",
			in:       `summary.record == "foo/results/bar/records/baz"`,
			wantSQL:  "(recordsummary_record = ?)",
			wantArgs: []any{"foo/results/bar/records/baz"},
		},
		{
			name:     "Result.Summary.StartTime field",
			in:       `summary.start_time > timestamp("2022/10/30T21:45:00.000Z")`,
			wantSQL:  "(recordsummary_start_time > ?::TIMESTAMP WITH TIME ZONE)",
			wantArgs: []any{"2022/10/30T21:45:00.000Z"},
		},
		{
			name:     "comparison with the PIPELINE_RUN const value",
			in:       `summary.type == PIPELINE_RUN`,
			wantSQL:  "(recordsummary_type = ?)",
			wantArgs: []any{"tekton.dev/v1.PipelineRun"},
		},
		{
			name:     "comparison with the TASK_RUN const value",
			in:       `summary.type == TASK_RUN`,
			wantSQL:  "(recordsummary_type = ?)",
			wantArgs: []any{"tekton.dev/v1.TaskRun"},
		},
		{
			name:     "RecordSummary_Status constants",
			in:       `summary.status == CANCELLED || summary.status == TIMEOUT`,
			wantSQL:  "((recordsummary_status = 4) OR (recordsummary_status = 3))",
			wantArgs: nil,
		},
		{
			name:     "Result.Summary.Annotations",
			in:       `summary.annotations["branch"] == "main"`,
			wantSQL:  `recordsummary_annotations @> ?::jsonb`,
			wantArgs: []any{`{"branch":"main"}`},
		},
		{
			name:     "not Result.Summary.Annotations",
			in:       `!(summary.annotations["branch"] == "main")`,
			wantSQL:  `NOT (recordsummary_annotations @> ?::jsonb)`,
			wantArgs: []any{`{"branch":"main"}`},
		},
		{
			name:     "Result.Summary.Annotations reversed operands",
			in:       `"main" == summary.annotations["branch"]`,
			wantSQL:  `recordsummary_annotations @> ?::jsonb`,
			wantArgs: []any{`{"branch":"main"}`},
		},
		{
			name:    "more complex expression",
			in:      `summary.annotations["actor"] == "john-doe" && summary.annotations["branch"] == "feat/amazing" && summary.status == SUCCESS`,
			wantSQL: `((recordsummary_annotations @> ?::jsonb AND recordsummary_annotations @> ?::jsonb) AND (recordsummary_status = 1))`,
			wantArgs: []any{
				`{"actor":"john-doe"}`,
				`{"branch":"feat/amazing"}`,
			},
		},
		// Injection regression tests
		{
			name:     "SQL injection via jsonb value is blocked",
			in:       `annotations["key"] == "val\") OR 1=1 --"`,
			wantSQL:  `annotations @> ?::jsonb`,
			wantArgs: []any{`{"key":"val\") OR 1=1 --"}`},
		},
	}

	env, err := cel.NewResultsEnv()
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotSQL, gotArgs, err := Convert(env, test.in)
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(test.wantSQL, gotSQL); diff != "" {
				t.Errorf("SQL mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.wantArgs, gotArgs); diff != "" {
				t.Errorf("args mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestConversionErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want error
	}{{
		name: "non-boolean expression",
		in:   "parent",
		want: errors.New("expected boolean expression, but got string"),
	},
	}

	env, err := cel.NewResultsEnv()
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _, err := Convert(env, test.in)
			if err == nil {
				t.Fatalf("Want the %q error, but the interpreter returned the following result instead: %q", test.want.Error(), got)
			}

			if diff := cmp.Diff(test.want.Error(), err.Error()); diff != "" {
				t.Fatalf("Mismatch in the error returned by the Convert function (-want +got):\n%s", diff)
			}
		})
	}
}
