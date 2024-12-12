package pipelinerun

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/tektoncd/results/pkg/test"

	"github.com/tektoncd/results/pkg/cli/flags"
	"github.com/tektoncd/results/pkg/test/fake"

	"github.com/jonboulle/clockwork"

	"github.com/spf13/cobra"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

func TestListPipelineRuns_empty(t *testing.T) {
	records := []*pb.Record{}
	now := time.Now()
	cmd := command(records, now)

	output, err := test.ExecuteCommand(cmd, "list")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	test.AssertOutput(t, "No PipelineRuns found\n", output)
}

func TestListPipelineRuns(t *testing.T) {
	clock := clockwork.NewFakeClock()
	createTime := clock.Now().Add(time.Duration(-3) * time.Minute)
	updateTime := clock.Now().Add(time.Duration(-2) * time.Minute)
	startTime := clock.Now().Add(time.Duration(-3) * time.Minute)
	endTime := clock.Now().Add(time.Duration(-1) * time.Minute)
	records := testDataSuccessfulPipelineRun(t, createTime, updateTime, startTime, endTime)
	cmd := command(records, clock.Now())
	output, err := test.ExecuteCommand(cmd, "list")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	test.AssertOutput(t, `NAMESPACE   UID                       STARTED         DURATION   STATUS
default     hello-goodbye-run-xgkf8   3 minutes ago   2m0s       Succeeded`, output)
}

func command(records []*pb.Record, now time.Time) *cobra.Command {
	clock := clockwork.NewFakeClockAt(now)

	param := &flags.Params{
		ResultsClient:    fake.NewResultsClient(nil, records),
		LogsClient:       nil,
		PluginLogsClient: nil,
		Clock:            clock,
	}
	cmd := Command(param)
	return cmd
}

func testDataSuccessfulPipelineRun(t *testing.T, createTime, updateTime, startTime, endTime time.Time) []*pb.Record {
	pipelineRun := &pipelinev1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "hello-goodbye-run-xgkf8",
			Namespace:       "default",
			UID:             "d2c19786-5fb7-4577-84a4-10d43c157c5c",
			ResourceVersion: "4320960",
			Generation:      1,
			Labels: map[string]string{
				"tekton.dev/pipeline": "hello-goodbye",
			},
			Annotations: map[string]string{
				"results.tekton.dev/log":    "default/results/d2c19786-5fb7-4577-84a4-10d43c157c5c/logs/d05a16ce-f7a4-3370-8c3a-88c30067680a",
				"results.tekton.dev/record": "default/results/d2c19786-5fb7-4577-84a4-10d43c157c5c/records/d2c19786-5fb7-4577-84a4-10d43c157c5c",
				"results.tekton.dev/result": "default/results/d2c19786-5fb7-4577-84a4-10d43c157c5c",
			},
			Finalizers: []string{"results.tekton.dev/pipelinerun"},
		},
		Spec: pipelinev1.PipelineRunSpec{
			PipelineRef: &pipelinev1.PipelineRef{
				Name: "hello-goodbye",
			},
			Params: []pipelinev1.Param{
				{
					Name: "username",
					Value: pipelinev1.ParamValue{
						Type:      pipelinev1.ParamTypeString,
						StringVal: "Tekton",
					},
				},
			},
			Timeouts: &pipelinev1.TimeoutFields{
				Pipeline: &metav1.Duration{Duration: time.Hour},
			},
		},
		Status: pipelinev1.PipelineRunStatus{
			PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
				StartTime:      &metav1.Time{Time: startTime},
				CompletionTime: &metav1.Time{Time: endTime},
				ChildReferences: []pipelinev1.ChildStatusReference{
					{
						Name:             "hello-goodbye-run-xgkf8-hello",
						PipelineTaskName: "hello",
					},
					{
						Name:             "hello-goodbye-run-xgkf8-goodbye",
						PipelineTaskName: "goodbye",
					},
				},
			},
			Status: duckv1.Status{
				Conditions: []apis.Condition{
					{
						Type:               apis.ConditionSucceeded,
						Status:             corev1.ConditionTrue,
						Reason:             "Succeeded",
						Message:            "Tasks Completed: 2 (Failed: 0, Cancelled 0), Skipped: 0",
						LastTransitionTime: apis.VolatileTime{Inner: metav1.Time{Time: updateTime}},
					},
				},
			},
		},
	}
	prBytes, err := json.Marshal(pipelineRun)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	records := []*pb.Record{
		{
			Name: "default/results/e6b4b2e3-d876-4bbe-a927-95c691b6fdc7/records/e6b4b2e3-d876-4bbe-a927-95c691b6fdc7",
			Uid:  "095a449f-691a-4be7-9bcb-3a52bba3bc6d",
			Data: &pb.Any{
				Type:  "tekton.dev/v1.PipelineRun",
				Value: prBytes,
			},
			CreateTime: timestamppb.New(createTime),
			UpdateTime: timestamppb.New(updateTime),
		},
	}
	return records
}
