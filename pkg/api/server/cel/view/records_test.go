package view

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/results/pkg/api/server/cel2sql"
	"k8s.io/utils/pointer"
)

func TestNewRecordsViewConstants(t *testing.T) {
	view, err := NewRecordsView()
	if err != nil {
		t.Fatal(err)
	}

	expectedConstants := map[string]cel2sql.Constant{
		"PIPELINE_RUN": {StringVal: pointer.String("tekton.dev/v1beta1.PipelineRun")},
		"TASK_RUN":     {StringVal: pointer.String("tekton.dev/v1beta1.TaskRun")},
	}
	if diff := cmp.Diff(expectedConstants, view.Constants); diff != "" {
		t.Errorf("Invalid constants (-want, +got):\n%s", diff)
	}
}
