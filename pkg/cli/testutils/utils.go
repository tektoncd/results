package testutils

import (
	"time"

	"github.com/jonboulle/clockwork"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	// Common test constants
	defaultNamespace     = "default"
	defaultConditionType = "Succeeded"
	pipelineRunKind      = "tekton.dev/v1.PipelineRun"
)

// TimePtr converts time.Time to *time.Time
func TimePtr(t time.Time) *time.Time {
	return &t
}

// CreateTestRecord creates a test record with all possible options
// Use empty string for namespace to use default, nil for labels to omit them
func CreateTestRecord(clock clockwork.Clock, name, uid, namespace string, startTime, endTime *time.Time, conditionStatus string, labels map[string]string) *pb.Record {
	// Use default namespace if not specified
	if namespace == "" {
		namespace = defaultNamespace
	}

	conditionType := defaultConditionType
	createTime := clock.Now().Add(-5 * time.Minute)

	var completionTimeJSON string
	if endTime != nil {
		completionTimeJSON = `"completionTime": "` + endTime.Format(time.RFC3339) + `",`
	}

	var startTimeJSON string
	if startTime != nil {
		startTimeJSON = `"startTime": "` + startTime.Format(time.RFC3339) + `",`
	}

	// Build labels JSON
	var labelsJSON string
	if len(labels) > 0 {
		labelsJSON = `,"labels": {`
		first := true
		for key, value := range labels {
			if !first {
				labelsJSON += ","
			}
			labelsJSON += `"` + key + `": "` + value + `"`
			first = false
		}
		labelsJSON += `}`
	}

	return &pb.Record{
		Name:       namespace + "/results/" + name + "/records/" + name,
		Uid:        "record-" + uid,
		CreateTime: timestamppb.New(createTime),
		Data: &pb.Any{
			Type: pipelineRunKind,
			Value: []byte(`{
				"apiVersion": "tekton.dev/v1",
				"kind": "PipelineRun",
				"metadata": {
					"name": "` + name + `",
					"namespace": "` + namespace + `",
					"uid": "` + uid + `"` + labelsJSON + `
				},
				"status": {
					` + startTimeJSON + `
					` + completionTimeJSON + `
					"conditions": [
						{
							"type": "` + conditionType + `",
							"status": "` + conditionStatus + `"
						}
					]
				}
			}`),
		},
	}
}
