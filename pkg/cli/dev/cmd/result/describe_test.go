package result

import (
	"testing"
	"time"

	"github.com/tektoncd/results/pkg/cli/testutils"

	"github.com/tektoncd/results/pkg/cli/dev/flags"

	"github.com/jonboulle/clockwork"
	"github.com/tektoncd/results/pkg/test"
	"github.com/tektoncd/results/pkg/test/fake"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestDescribeResult(t *testing.T) {
	clock := clockwork.NewFakeClock()
	createTime := clock.Now().Add(time.Duration(-3) * time.Minute)
	updateTime := clock.Now().Add(time.Duration(-2) * time.Minute)
	startTime := clock.Now().Add(time.Duration(-3) * time.Minute)
	endTime := clock.Now().Add(time.Duration(-1) * time.Minute)
	results := []*pb.Result{
		{
			Name:       "default/results/e6b4b2e3-d876-4bbe-a927-95c691b6fdc7",
			Uid:        "949eebd9-1cf7-478f-a547-9ee313035f10",
			CreateTime: timestamppb.New(createTime),
			UpdateTime: timestamppb.New(updateTime),
			Annotations: map[string]string{
				"object.metadata.name": "hello-goodbye-run-vfsxn",
				"tekton.dev/pipeline":  "hello-goodbye",
			},
			Summary: &pb.RecordSummary{
				Record:    "default/results/e6b4b2e3-d876-4bbe-a927-95c691b6fdc7/records/e6b4b2e3-d876-4bbe-a927-95c691b6fdc7",
				Type:      "tekton.dev/v1.PipelineRun",
				StartTime: timestamppb.New(startTime),
				EndTime:   timestamppb.New(endTime),
				Status:    pb.RecordSummary_SUCCESS,
			},
		}}

	param := &flags.Params{
		ResultsClient:    fake.NewResultsClient(results, nil),
		LogsClient:       nil,
		PluginLogsClient: nil,
		Clock:            clock,
	}
	cmd := Command(param)

	output, err := testutils.ExecuteCommand(cmd, "describe", "default/results/e6b4b2e3-d876-4bbe-a927-95c691b6fdc7")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	test.AssertOutput(t, `Name:   default/results/e6b4b2e3-d876-4bbe-a927-95c691b6fdc7
UID:    949eebd9-1cf7-478f-a547-9ee313035f10
Annotations:
	object.metadata.name=hello-goodbye-run-vfsxn
	tekton.dev/pipeline=hello-goodbye
Status:
	Created:   3 minutes ago   DURATION: 1m0s
Summary:
	Type:   tekton.dev/v1.PipelineRun
	Status:
	STARTED         DURATION   STATUS
	3 minutes ago   2m0s       SUCCESS
`, output)
}
