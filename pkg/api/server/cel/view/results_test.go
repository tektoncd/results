package view

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/results/pkg/api/server/cel2sql"
	"k8s.io/utils/pointer"
)

func TestNewResultsViewConstants(t *testing.T) {
	view, err := NewResultsView()
	if err != nil {
		t.Fatal(err)
	}

	expectedConstants := map[string]cel2sql.Constant{
		"PIPELINE_RUN": {StringVal: pointer.String("tekton.dev/v1beta1.PipelineRun")},
		"TASK_RUN":     {StringVal: pointer.String("tekton.dev/v1beta1.TaskRun")},
		"UNKNOWN":      {Int32Val: pointer.Int32(0)},
		"SUCCESS":      {Int32Val: pointer.Int32(1)},
		"FAILURE":      {Int32Val: pointer.Int32(2)},
		"TIMEOUT":      {Int32Val: pointer.Int32(3)},
		"CANCELLED":    {Int32Val: pointer.Int32(4)},
	}
	if diff := cmp.Diff(expectedConstants, view.Constants); diff != "" {
		t.Errorf("Invalid constants (-want, +got):\n%s", diff)
	}
}
