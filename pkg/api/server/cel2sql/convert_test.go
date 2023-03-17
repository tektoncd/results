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
		name string
		in   string
		want string
	}{
		{
			name: "simple expression",
			in:   `name == "foo"`,
			want: "name = 'foo'",
		},
		{
			name: "select expression",
			in:   `data.metadata.namespace == "default"`,
			want: "(data->'metadata'->>'namespace') = 'default'",
		},
		{
			name: "type coercion with a dyn expression in the left hand side",
			in:   `data.status.completionTime > timestamp("2022/10/30T21:45:00.000Z")`,
			want: "(data->'status'->>'completionTime')::TIMESTAMP WITH TIME ZONE > '2022/10/30T21:45:00.000Z'::TIMESTAMP WITH TIME ZONE",
		},
		{
			name: "type coercion with a dyn expression in the right hand side",
			in:   `timestamp("2022/10/30T21:45:00.000Z") < data.status.completionTime`,
			want: "'2022/10/30T21:45:00.000Z'::TIMESTAMP WITH TIME ZONE < (data->'status'->>'completionTime')::TIMESTAMP WITH TIME ZONE",
		},
		{
			name: "in operator",
			in:   `data.metadata.namespace in ["foo", "bar"]`,
			want: "(data->'metadata'->>'namespace') IN ('foo', 'bar')",
		},
		{
			name: "index operator",
			in:   `data.metadata.labels["foo"] == "bar"`,
			want: "(data->'metadata'->'labels'->>'foo') = 'bar'",
		},
		{
			name: "concatenate strings",
			in:   `name + "bar" == "foobar"`,
			want: "CONCAT(name, 'bar') = 'foobar'",
		},
		{
			name: "multiple concatenate strings",
			in:   `name + "bar" + "baz" == "foobarbaz"`,
			want: "CONCAT(name, 'bar', 'baz') = 'foobarbaz'",
		},
		{
			name: "contains string function",
			in:   `data.metadata.name.contains("foo")`,
			want: "POSITION('foo' IN (data->'metadata'->>'name')) <> 0",
		},
		{
			name: "endsWith string function",
			in:   `data.metadata.name.endsWith("bar")`,
			want: "(data->'metadata'->>'name') LIKE '%' || 'bar'",
		},
		{
			name: "getDate function",
			in:   `data.status.completionTime.getDate() == 2`,
			want: "EXTRACT(DAY FROM (data->'status'->>'completionTime')::TIMESTAMP WITH TIME ZONE) = 2",
		},
		{
			name: "getDayOfMonth function",
			in:   `data.status.completionTime.getDayOfMonth() == 2`,
			want: "(EXTRACT(DAY FROM (data->'status'->>'completionTime')::TIMESTAMP WITH TIME ZONE) - 1) = 2",
		},
		{
			name: "getDayOfWeek function",
			in:   `data.status.completionTime.getDayOfWeek() > 0`,
			want: "EXTRACT(DOW FROM (data->'status'->>'completionTime')::TIMESTAMP WITH TIME ZONE) > 0",
		},
		{
			name: "getDayOfYear function",
			in:   `data.status.completionTime.getDayOfYear() > 15`,
			want: "(EXTRACT(DOY FROM (data->'status'->>'completionTime')::TIMESTAMP WITH TIME ZONE) - 1) > 15",
		},
		{
			name: "getFullYear function",
			in:   `data.status.completionTime.getFullYear() >= 2022`,
			want: "EXTRACT(YEAR FROM (data->'status'->>'completionTime')::TIMESTAMP WITH TIME ZONE) >= 2022",
		},
		{
			name: "matches function",
			in:   `data.metadata.name.matches("^foo.*$")`,
			want: "(data->'metadata'->>'name') ~ '^foo.*$'",
		},
		{
			name: "startsWith string function",
			in:   `data.metadata.name.startsWith("bar")`,
			want: "(data->'metadata'->>'name') LIKE 'bar' || '%'",
		},
		{
			name: "data_type field",
			in:   `data_type == PIPELINE_RUN`,
			want: "type = 'tekton.dev/v1beta1.PipelineRun'",
		},
		{
			name: "index operator with numeric argument in JSON arrays",
			in:   `data_type == "tekton.dev/v1beta1.TaskRun" && data.status.conditions[0].status == "True"`,
			want: "type = 'tekton.dev/v1beta1.TaskRun' AND (data->'status'->'conditions'->0->>'status') = 'True'",
		},
		{
			name: "index operator as first operation in JSON object",
			in:   `data_type == "tekton.dev/v1beta1.TaskRun" && data["status"].conditions[0].status == "True"`,
			want: "type = 'tekton.dev/v1beta1.TaskRun' AND (data->'status'->'conditions'->0->>'status') = 'True'",
		},
		{
			name: "index operator with string argument in JSON object",
			in:   `data_type == "tekton.dev/v1beta1.TaskRun" && data.status["conditions"][0].status == "True"`,
			want: "type = 'tekton.dev/v1beta1.TaskRun' AND (data->'status'->'conditions'->0->>'status') = 'True'",
		},
	}

	env, err := cel.NewRecordsEnv()
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Convert(env, test.in)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("want: %+v\n", test.want)
			t.Logf("got:  %+v\n", got)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("Mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestConvertResultExpressions(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{{
		name: "Result.Parent field",
		in:   `parent.endsWith("bar")`,
		want: "parent LIKE '%' || 'bar'",
	},
		{
			name: "Result.Uid field",
			in:   `uid == "foo"`,
			want: "id = 'foo'",
		},
		{
			name: "Result.Annotations field",
			in:   `annotations["repo"] == "tektoncd/results"`,
			want: `annotations @> '{"repo":"tektoncd/results"}'::jsonb`,
		},
		{
			name: "Result.Annotations field",
			in:   `"tektoncd/results" == annotations["repo"]`,
			want: `annotations @> '{"repo":"tektoncd/results"}'::jsonb`,
		},
		{
			name: "other operators involving the Result.Annotations field",
			in:   `annotations["repo"].startsWith("tektoncd")`,
			want: "annotations->>'repo' LIKE 'tektoncd' || '%'",
		},
		{
			name: "Result.CreateTime field",
			in:   `create_time > timestamp("2022/10/30T21:45:00.000Z")`,
			want: "created_time > '2022/10/30T21:45:00.000Z'::TIMESTAMP WITH TIME ZONE",
		},
		{
			name: "Result.UpdateTime field",
			in:   `update_time > timestamp("2022/10/30T21:45:00.000Z")`,
			want: "updated_time > '2022/10/30T21:45:00.000Z'::TIMESTAMP WITH TIME ZONE",
		},
		{
			name: "Result.Summary.Record field",
			in:   `summary.record == "foo/results/bar/records/baz"`,
			want: "recordsummary_record = 'foo/results/bar/records/baz'",
		},
		{
			name: "Result.Summary.StartTime field",
			in:   `summary.start_time > timestamp("2022/10/30T21:45:00.000Z")`,
			want: "recordsummary_start_time > '2022/10/30T21:45:00.000Z'::TIMESTAMP WITH TIME ZONE",
		},
		{
			name: "comparison with the PIPELINE_RUN const value",
			in:   `summary.type == PIPELINE_RUN`,
			want: "recordsummary_type = 'tekton.dev/v1beta1.PipelineRun'",
		},
		{
			name: "comparison with the TASK_RUN const value",
			in:   `summary.type == TASK_RUN`,
			want: "recordsummary_type = 'tekton.dev/v1beta1.TaskRun'",
		},
		{
			name: "RecordSummary_Status constants",
			in:   `summary.status == CANCELLED || summary.status == TIMEOUT`,
			want: "recordsummary_status = 4 OR recordsummary_status = 3",
		},
		{
			name: "Result.Summary.Annotations",
			in:   `summary.annotations["branch"] == "main"`,
			want: `recordsummary_annotations @> '{"branch":"main"}'::jsonb`,
		},
		{
			name: "Result.Summary.Annotations",
			in:   `"main" == summary.annotations["branch"]`,
			want: `recordsummary_annotations @> '{"branch":"main"}'::jsonb`,
		},
		{
			name: "more complex expression",
			in:   `summary.annotations["actor"] == "john-doe" && summary.annotations["branch"] == "feat/amazing" && summary.status == SUCCESS`,
			want: `recordsummary_annotations @> '{"actor":"john-doe"}'::jsonb AND recordsummary_annotations @> '{"branch":"feat/amazing"}'::jsonb AND recordsummary_status = 1`,
		},
	}

	env, err := cel.NewResultsEnv()
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Convert(env, test.in)
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("Mismatch (-want +got):\n%s", diff)
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
			got, err := Convert(env, test.in)
			if err == nil {
				t.Fatalf("Want the %q error, but the interpreter returned the following result instead: %q", test.want.Error(), got)
			}

			if diff := cmp.Diff(test.want.Error(), err.Error()); diff != "" {
				t.Fatalf("Mismatch in the error returned by the Convert function (-want +got):\n%s", diff)
			}
		})
	}
}
