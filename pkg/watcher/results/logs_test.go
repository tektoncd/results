package results

import (
	"context"
	"fmt"
	"testing"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClient_PutLog(t *testing.T) {
	ctx := context.Background()
	c := client(t)

	objs := []Object{
		&pipelinev1.TaskRun{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "tekton.dev/v1",
				Kind:       "TaskRun",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "taskrun",
				Namespace: "test",
				UID:       "taskrun-id",
			},
		},
		&pipelinev1.PipelineRun{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "tekton.dev/v1",
				Kind:       "PipelineRun",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pipelinerun",
				Namespace: "test",
				UID:       "pipelinerun-id",
			},
		},
	}
	for _, o := range objs {
		t.Run(o.GetName(), func(t *testing.T) {
			for _, tc := range []string{"create", "update"} {
				t.Run(tc, func(t *testing.T) {
					if _, err := c.PutLog(ctx, o); err != nil {
						t.Fatal(err)
					}
				})
			}

			// Verify Result/Record exist.
			res, err := c.GetResult(ctx, &pb.GetResultRequest{
				Name: fmt.Sprintf("test/results/%s", o.GetUID()),
			})
			if err != nil {
				t.Fatalf("GetResult: %v", err)
			}
			name, err := getLogRecordName(res, o)
			if err != nil {
				t.Fatalf("Error getting record name: %v", err)
			}
			if _, err := c.GetRecord(ctx, &pb.GetRecordRequest{
				Name: name,
			}); err != nil {
				t.Fatalf("GetRecord: %v", err)
			}
		})
	}
}
