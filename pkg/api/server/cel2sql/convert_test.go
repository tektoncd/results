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

	"github.com/google/cel-go/cel"
	"github.com/google/go-cmp/cmp"
	resultspb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

func newTestView() *View {
	typePipelineRun := "tekton.dev/v1beta1.PipelineRun"
	return &View{
		Constants: map[string]Constant{
			"PIPELINE_RUN": {
				StringVal: &typePipelineRun,
			},
		},
		Fields: map[string]Field{
			"parent": {
				CELType: cel.StringType,
				SQL:     `parent`,
			},
			"create_time": {
				CELType: CELTypeTimestamp,
				SQL:     `created_time`,
			},
			"annotations": {
				CELType: cel.MapType(cel.StringType, cel.StringType),
				SQL:     `annotations`,
			},
			"summary": {
				CELType:    cel.ObjectType("tekton.results.v1alpha2.RecordSummary"),
				ObjectType: &resultspb.RecordSummary{},
				SQL:        `recordsummary_{{.Field}}`,
			},
			"data": {
				CELType: cel.AnyType,
				SQL:     `data`,
			},
		},
	}
}
func TestConversionErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want error
	}{
		{
			name: "compile error missing field",
			in:   "parnt",
			want: errors.New("error compiling CEL filters: ERROR: <input>:1:1: undeclared reference to 'parnt' (in container '')\n | parnt\n | ^"),
		},
		{
			name: "non-boolean expression",
			in:   "parent",
			want: errors.New("expected boolean expression, but got string"),
		},
	}

	view := newTestView()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Convert(view, test.in)
			if err == nil {
				t.Fatalf("Want the %q error, but the interpreter returned the following result instead: %q", test.want.Error(), got)
			}

			if diff := cmp.Diff(test.want.Error(), err.Error()); diff != "" {
				t.Fatalf("Mismatch in the error returned by the Convert function (-want +got):\n%s", diff)
			}
		})
	}
}

func TestConvert(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		// About operations
		{
			name: "able to match strings exactly",
			in:   `parent == "foo"`,
			want: "parent = 'foo'",
		},
		{
			name: "able to use endsWith function",
			in:   `parent.endsWith("bar")`,
			want: "parent LIKE '%' || 'bar'",
		},
		{
			name: "able to use in operator",
			in:   `parent in ["foo", "bar"]`,
			want: "parent IN ('foo', 'bar')",
		},
		{
			name: "able to select values from any type",
			in:   `data.metadata.namespace == "default"`,
			want: "(data->'metadata'->>'namespace') = 'default'",
		},
		{
			name: "able to use index operator to navigate maps",
			in:   `data.metadata.labels["foo"] == "bar"`,
			want: "(data->'metadata'->'labels'->>'foo') = 'bar'",
		},
		{
			name: "able to use index operator to navigate arrays",
			in:   `data.metadata.ownerReferences[0].name == "bar"`,
			want: "(data->'metadata'->'ownerReferences'->0->>'name') = 'bar'",
		},
		{
			name: "able to use index operator in first selection in any type",
			in:   `data["status"].conditions[0].status == "True"`,
			want: "(data->'status'->'conditions'->0->>'status') = 'True'",
		},
		{
			name: "able to chain index operators",
			in:   `data.status["conditions"][0].status == "True"`,
			want: "(data->'status'->'conditions'->0->>'status') = 'True'",
		},
		{
			name: "able to concatenate strings",
			in:   `parent + "bar" == "foobar"`,
			want: "CONCAT(parent, 'bar') = 'foobar'",
		},
		{
			name: "able to concatenate multiple concatenate strings",
			in:   `parent + "bar" + "baz" == "foobarbaz"`,
			want: "CONCAT(parent, 'bar', 'baz') = 'foobarbaz'",
		},
		{
			name: "able to use contains string function",
			in:   `parent.contains("foo")`,
			want: "POSITION('foo' IN parent) <> 0",
		},
		{
			name: "able to use matches function",
			in:   `parent.matches("^foo.*$")`,
			want: "parent ~ '^foo.*$'",
		},
		// About maps
		{
			name: "able to match map of strings in left hand side",
			in:   `annotations["repo"] == "tektoncd/results"`,
			want: `annotations @> '{"repo":"tektoncd/results"}'::jsonb`,
		},
		{
			name: "able to match map of strings in right hand side",
			in:   `"tektoncd/results" == annotations["repo"]`,
			want: `annotations @> '{"repo":"tektoncd/results"}'::jsonb`,
		},
		{
			name: "able to use functions after accessing map values",
			in:   `annotations["repo"].startsWith("tektoncd")`,
			want: "annotations->>'repo' LIKE 'tektoncd' || '%'",
		},
		// About timestamp
		{
			name: "able to filter timestamp",
			in:   `create_time > timestamp("2022/10/30T21:45:00.000Z")`,
			want: "created_time > '2022/10/30T21:45:00.000Z'::TIMESTAMP WITH TIME ZONE",
		},
		{
			name: "able to perform type coercion with a dyn expression in the left hand side",
			in:   `data.status.completionTime > timestamp("2022/10/30T21:45:00.000Z")`,
			want: "(data->'status'->>'completionTime')::TIMESTAMP WITH TIME ZONE > '2022/10/30T21:45:00.000Z'::TIMESTAMP WITH TIME ZONE",
		},
		{
			name: "able to perform type coercion with a dyn expression in the right hand side",
			in:   `timestamp("2022/10/30T21:45:00.000Z") < data.status.completionTime`,
			want: "'2022/10/30T21:45:00.000Z'::TIMESTAMP WITH TIME ZONE < (data->'status'->>'completionTime')::TIMESTAMP WITH TIME ZONE",
		},
		{
			name: "able to use getDate function",
			in:   `data.status.completionTime.getDate() == 2`,
			want: "EXTRACT(DAY FROM (data->'status'->>'completionTime')::TIMESTAMP WITH TIME ZONE) = 2",
		},
		{
			name: "able to use getDayOfMonth function",
			in:   `data.status.completionTime.getDayOfMonth() == 2`,
			want: "(EXTRACT(DAY FROM (data->'status'->>'completionTime')::TIMESTAMP WITH TIME ZONE) - 1) = 2",
		},
		{
			name: "able to use getDayOfWeek function",
			in:   `data.status.completionTime.getDayOfWeek() > 0`,
			want: "EXTRACT(DOW FROM (data->'status'->>'completionTime')::TIMESTAMP WITH TIME ZONE) > 0",
		},
		{
			name: "able to use getDayOfYear function",
			in:   `data.status.completionTime.getDayOfYear() > 15`,
			want: "(EXTRACT(DOY FROM (data->'status'->>'completionTime')::TIMESTAMP WITH TIME ZONE) - 1) > 15",
		},
		{
			name: "able to use getFullYear function",
			in:   `data.status.completionTime.getFullYear() >= 2022`,
			want: "EXTRACT(YEAR FROM (data->'status'->>'completionTime')::TIMESTAMP WITH TIME ZONE) >= 2022",
		},
		// About objects
		{
			name: "able to map object fields to columns",
			in:   `summary.record == "foo/results/bar/baz"`,
			want: "recordsummary_record = 'foo/results/bar/baz'",
		},
		// About constants
		{
			name: "able to compare with const value",
			in:   `summary.type == PIPELINE_RUN`,
			want: "recordsummary_type = 'tekton.dev/v1beta1.PipelineRun'",
		},
		// About compatibility of features
		{
			name: "able to use map of strings inside objects in the left hand side",
			in:   `summary.annotations["branch"] == "main"`,
			want: `recordsummary_annotations @> '{"branch":"main"}'::jsonb`,
		},
		{
			name: "able to use map of strings inside objects in the right hand side",
			in:   `"main" == summary.annotations["branch"]`,
			want: `recordsummary_annotations @> '{"branch":"main"}'::jsonb`,
		},
		{
			name: "more complex expression",
			in:   `summary.annotations["actor"] == "john-doe" && summary.annotations["branch"] == "feat/amazing" && summary.type == PIPELINE_RUN`,
			want: `recordsummary_annotations @> '{"actor":"john-doe"}'::jsonb AND recordsummary_annotations @> '{"branch":"feat/amazing"}'::jsonb AND recordsummary_type = 'tekton.dev/v1beta1.PipelineRun'`,
		},
	}

	view := newTestView()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Convert(view, test.in)
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("Mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
