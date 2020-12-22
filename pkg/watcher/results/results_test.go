package results

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/results/pkg/watcher/convert"
	"github.com/tektoncd/results/pkg/watcher/internal/test"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDefaultName(t *testing.T) {
	// No need to create a full client - we're only testing a utility method.
	client := &Client{kind: "test"}
	want := "test-name"

	objs := []metav1.Object{
		&v1beta1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "name",
				Namespace: "test",
			},
		},
		&v1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "name",
				Namespace: "test",
			},
		},
	}
	for _, o := range objs {
		t.Run(fmt.Sprintf("%T", o), func(t *testing.T) {
			if got := client.defaultName(o); want != got {
				t.Errorf("want %s, got %s", want, got)
			}
		})
	}
}

func TestEnsureResult(t *testing.T) {
	ctx := context.Background()
	client := client(t)

	objs := []metav1.Object{
		&v1beta1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "taskrun",
				Namespace: "test",
			},
		},
		&v1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pipelinerun",
				Namespace: "test",
			},
		},
	}
	for _, o := range objs {
		name := fmt.Sprintf("test/results/test-%s", o.GetName())

		// Sanity check Result doesn't exist.
		if r, err := client.GetResult(ctx, &pb.GetResultRequest{Name: name}); status.Code(err) != codes.NotFound {
			t.Fatalf("Result already exists: %+v", r)
		}

		// Run each test 2x - once for the initial Result creation, another to
		// get the existing Result.
		for _, tc := range []string{"create", "get"} {
			t.Run(tc, func(t *testing.T) {
				got, err := client.ensureResult(ctx, o)
				if err != nil {
					t.Fatal(err)
				}
				want := &pb.Result{
					Name: name,
				}
				if diff := cmp.Diff(want, got, protocmp.Transform(), protocmp.IgnoreFields(want, "id", "created_time", "updated_time")); diff != "" {
					t.Errorf("Result diff (-want, +got):\n%s", diff)
				}
			})
		}
	}
}

func TestUpsertRecord(t *testing.T) {
	ctx := context.Background()
	client := client(t)

	objs := []metav1.Object{
		&v1beta1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "taskrun",
				Namespace: "test",
			},
		},
		&v1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pipelinerun",
				Namespace: "test",
			},
		},
	}
	for _, o := range objs {
		result, err := client.ensureResult(ctx, o)
		if err != nil {
			t.Fatal(err)
		}

		name := fmt.Sprintf("%s/records/test-%s", result.GetName(), o.GetName())

		// Sanity check Record doesn't exist
		if r, err := client.GetRecord(ctx, &pb.GetRecordRequest{Name: name}); status.Code(err) != codes.NotFound {
			t.Fatalf("Record already exists: %+v", r)
		}

		// Run each test 2x - once for the initial Record creation, another to
		// update the existing Record.
		for _, tc := range []string{"create", "update"} {
			t.Run(tc, func(t *testing.T) {
				got, err := client.upsertRecord(ctx, result.GetName(), o)
				if err != nil {
					t.Fatalf("upsertRecord: %v", err)
				}
				want := crdToRecord(t, name, o)
				opts := []cmp.Option{protocmp.Transform(), protocmp.IgnoreFields(want, "id")}
				if diff := cmp.Diff(want, got, opts...); diff != "" {
					t.Errorf("upsertRecord diff (-want, +got):\n%s", diff)
				}

				// Verify upstream Record matches.
				got, err = client.GetRecord(ctx, &pb.GetRecordRequest{Name: name})
				if err != nil {
					t.Fatalf("GetRecord: %v", err)
				}
				if diff := cmp.Diff(want, got, opts...); diff != "" {
					t.Errorf("GetRecord diff (-want, +got):\n%s", diff)
				}
			})
		}
	}
}

func TestPut(t *testing.T) {
	ctx := context.Background()
	client := client(t)

	objs := []metav1.Object{
		&v1beta1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "taskrun",
				Namespace: "test",
			},
		},
		&v1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pipelinerun",
				Namespace: "test",
			},
		},
	}
	for _, o := range objs {
		// Run each test 2x - once for the initial creation, another to
		// simulate an update.
		// This is less exhaustive than the other tests, since Put is a wrapper
		// around ensureResult/upsertRecord.
		for _, tc := range []string{"create", "update"} {
			t.Run(tc, func(t *testing.T) {
				if _, _, err := client.Put(ctx, o); err != nil {
					t.Fatal(err)
				}
			})
		}

		// Verify Result/Record exist.
		if _, err := client.GetResult(ctx, &pb.GetResultRequest{
			Name: fmt.Sprintf("test/results/test-%s", o.GetName()),
		}); err != nil {
			t.Fatalf("GetResult: %v", err)
		}
		if _, err := client.GetRecord(ctx, &pb.GetRecordRequest{
			Name: fmt.Sprintf("test/results/test-%s/records/test-%s", o.GetName(), o.GetName()),
		}); err != nil {
			t.Fatalf("GetRecord: %v", err)
		}
	}
}

func crdToRecord(t *testing.T, name string, o metav1.Object) *pb.Record {
	t.Helper()

	m, err := convert.ToProto(o)
	if err != nil {
		t.Fatalf("convert.ToProto(): %v", err)
	}
	return &pb.Record{
		Name: name,
		Data: m,
	}
}

func client(t *testing.T) *Client {
	t.Helper()

	return &Client{
		ResultsClient: test.NewResultsClient(t),
		kind:          "test",
	}
}
