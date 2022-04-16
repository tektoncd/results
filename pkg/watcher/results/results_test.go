// Copyright 2021 The Tekton Authors
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

package results

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/results/pkg/internal/protoutil"
	"github.com/tektoncd/results/pkg/internal/test"
	"github.com/tektoncd/results/pkg/watcher/convert"
	"github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logtest "knative.dev/pkg/logging/testing"
)

func TestDefaultName(t *testing.T) {
	want := "id"

	objs := []metav1.Object{
		&v1beta1.TaskRun{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "tekton.dev/v1beta1",
				Kind:       "TaskRun",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "name",
				Namespace: "test",
				UID:       "id",
			},
		},
		&v1beta1.PipelineRun{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "tekton.dev/v1beta1",
				Kind:       "PipelineRun",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "name",
				Namespace: "test",
				UID:       "id",
			},
		},
	}
	for _, o := range objs {
		t.Run(fmt.Sprintf("%T", o), func(t *testing.T) {
			if got := defaultName(o); want != got {
				t.Errorf("want %s, got %s", want, got)
			}
		})
	}
}

func TestResultName(t *testing.T) {
	ownerRef := []metav1.OwnerReference{{
		Kind: "PipelineRun",
		UID:  "pipelinerun",
	}}

	for _, tc := range []struct {
		name        string
		modify      func(o metav1.Object)
		annotations map[string]string
		want        string
	}{
		{
			name: "object name",
			want: "test/results/id",
		},
		{
			name: "pipeline run",
			modify: func(o metav1.Object) {
				o.SetOwnerReferences(ownerRef)
			},
			want: "test/results/pipelinerun",
		},
		{
			name: "trigger event",
			modify: func(o metav1.Object) {
				o.SetOwnerReferences(ownerRef)
				o.SetLabels(map[string]string{
					"triggers.tekton.dev/triggers-eventid": "trigger",
				})
			},
			want: "test/results/trigger",
		},
		{
			name: "result",
			modify: func(o metav1.Object) {
				o.SetOwnerReferences(ownerRef)
				o.SetLabels(map[string]string{
					"triggers.tekton.dev/triggers-eventid": "trigger",
				})
				o.SetAnnotations(map[string]string{
					annotation.Result: "result",
				})
			},
			// This is not modified, since we assume that results are referred
			// to by the full name already.
			want: "result",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := &v1beta1.TaskRun{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "tekton.dev/v1beta1",
					Kind:       "TaskRun",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "object",
					Namespace: "test",
					UID:       "id",
				},
			}
			if tc.modify != nil {
				tc.modify(o)
			}
			if got := resultName(o); tc.want != got {
				t.Errorf("want %s, got %s", tc.want, got)
			}
		})
	}
}

func TestEnsureResult(t *testing.T) {
	ctx := logtest.TestContextWithLogger(t)
	client := client(t)

	objs := []Object{
		&v1beta1.TaskRun{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "tekton.dev/v1beta1",
				Kind:       "TaskRun",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "taskrun",
				Namespace: "test",
				UID:       "taskrun-id",
			},
		},
		&v1beta1.PipelineRun{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "tekton.dev/v1beta1",
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
		name := fmt.Sprintf("test/results/%s", o.GetUID())

		// Sanity check Result doesn't exist.
		if r, err := client.GetResult(ctx, &pb.GetResultRequest{Name: name}); status.Code(err) != codes.NotFound {
			t.Fatalf("Result already exists: %+v", r)
		}

		// Run each test 2x - once for the initial Result creation, another to
		// get the existing Result.
		t.Run(o.GetName(), func(t *testing.T) {
			create, err := client.ensureResult(ctx, o)
			if err != nil {
				t.Fatal(err)
			}
			want := &pb.Result{
				Name: name,
				Summary: &pb.RecordSummary{
					Record: recordName(name, o),
					Type:   convert.TypeName(o),
				},
			}
			if diff := cmp.Diff(want, create, protocmp.Transform(), protoutil.IgnoreResultOutputOnly()); diff != "" {
				t.Errorf("Create Result diff (-want, +got):\n%s", diff)
			}

			get, err := client.ensureResult(ctx, o)
			if err != nil {
				t.Fatal(err)
			}
			// Don't ignore the OUTPUT_ONLY fields this time - the object
			// should be identical to the Result created before.
			if diff := cmp.Diff(create, get, protocmp.Transform()); diff != "" {
				t.Errorf("Get Result diff (-want, +got):\n%s", diff)
			}

		})
	}
}

func TestEnsureResult_RecordSummaryUpdate(t *testing.T) {
	ctx := logtest.TestContextWithLogger(t)
	client := client(t)

	pr := &v1beta1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			UID:       "1",
		},
	}
	tr := &v1beta1.TaskRun{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       pr.Namespace,
			UID:             "2",
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(pr, pr.GetGroupVersionKind())},
		},
	}

	// Create TaskRun first - this will create a Result for the PipelineRun,
	// but will *not* populate the RecordSummary.
	got, err := client.ensureResult(ctx, tr)
	if err != nil {
		t.Fatal(err)
	}
	want := &pb.Result{Name: resultName(pr)}
	if diff := cmp.Diff(got, want, protocmp.Transform(), protoutil.IgnoreResultOutputOnly()); diff != "" {
		t.Fatal(diff)
	}

	// Create the PipelineRun - this will update the Result with the Summary.
	got, err = client.ensureResult(ctx, pr)
	if err != nil {
		t.Fatal(err)
	}
	want = &pb.Result{
		Name: resultName(pr),
		Summary: &pb.RecordSummary{
			Record: recordName(resultName(pr), pr),
			Type:   convert.TypeName(pr),
		},
	}
	if diff := cmp.Diff(got, want, protocmp.Transform(), protoutil.IgnoreResultOutputOnly()); diff != "" {
		t.Fatal(diff)
	}
}

func TestUpsertRecord(t *testing.T) {
	ctx := context.Background()
	client := client(t)

	objs := []Object{
		&v1beta1.TaskRun{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "tekton.dev/v1beta1",
				Kind:       "TaskRun",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:       "taskrun",
				Namespace:  "test",
				UID:        "taskrun-id",
				Generation: 1,
			},
			Status: v1beta1.TaskRunStatus{
				TaskRunStatusFields: v1beta1.TaskRunStatusFields{
					TaskRunResults: []v1beta1.TaskRunResult{
						{
							Name:  "result1",
							Value: "value1",
						},
					},
				},
			},
		},
		&v1beta1.PipelineRun{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "tekton.dev/v1beta1",
				Kind:       "PipelineRun",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:       "pipelinerun",
				Namespace:  "test",
				UID:        "pipelinerun-id",
				Generation: 1,
			},
			Status: v1beta1.PipelineRunStatus{
				PipelineRunStatusFields: v1beta1.PipelineRunStatusFields{
					PipelineResults: []v1beta1.PipelineRunResult{
						{
							Name:  "result1",
							Value: "value1",
						},
					},
				},
			},
		},
	}
	for _, o := range objs {
		t.Run(o.GetName(), func(t *testing.T) {
			result, err := client.ensureResult(ctx, o)
			if err != nil {
				t.Fatal(err)
			}

			// Sanity check Record doesn't exist
			name := fmt.Sprintf("%s/records/%s", result.GetName(), o.GetUID())
			if r, err := client.GetRecord(ctx, &pb.GetRecordRequest{Name: name}); status.Code(err) != codes.NotFound {
				t.Fatalf("Record already exists: %+v", r)
			}

			// Ignore server generated fields.
			opts := []cmp.Option{protocmp.Transform(), protocmp.IgnoreFields(&pb.Record{}, "id", "uid", "updated_time", "update_time", "create_time", "created_time", "etag")}

			var record *pb.Record
			// Start from scratch and create a new record.
			t.Run("create", func(t *testing.T) {
				record, err = client.upsertRecord(ctx, result.GetName(), o)
				if err != nil {
					t.Fatalf("upsertRecord: %v", err)
				}
				want := crdToRecord(t, name, o)
				if diff := cmp.Diff(want, record, opts...); diff != "" {
					t.Errorf("upsertRecord diff (-want, +got):\n%s", diff)
				}
				// Verify upstream Record matches.
				got, err := client.GetRecord(ctx, &pb.GetRecordRequest{Name: name})
				if err != nil {
					t.Fatalf("GetRecord: %v", err)
				}
				if diff := cmp.Diff(want, got, opts...); diff != "" {
					t.Errorf("GetRecord diff (-want, +got):\n%s", diff)
				}
			})

			// Attempt to update the record as-is. Since there is no diff there
			// should not be an update - we should get the same object back.
			t.Run("no-op", func(t *testing.T) {
				got, err := client.upsertRecord(ctx, result.GetName(), o)
				if err != nil {
					t.Fatalf("upsertRecord: %v", err)
				}

				if diff := cmp.Diff(record, got, protocmp.Transform()); diff != "" {
					t.Errorf("upsertRecord diff (-want, +got):\n%s", diff)
				}
			})

			// Modify object to cause a diff + actual update.
			t.Run("update", func(t *testing.T) {
				// Update the result on each TaskRun and PipelineRun.
				var updated Object
				if o.GetName() == "taskrun" {
					updated = &v1beta1.TaskRun{
						TypeMeta: metav1.TypeMeta{
							APIVersion: "tekton.dev/v1beta1",
							Kind:       "TaskRun",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:       "taskrun",
							Namespace:  "test",
							UID:        "taskrun-id",
							Generation: 1,
						},
						Status: v1beta1.TaskRunStatus{
							TaskRunStatusFields: v1beta1.TaskRunStatusFields{
								TaskRunResults: []v1beta1.TaskRunResult{
									{
										Name:  "result1",
										Value: "value1-updated",
									},
								},
							},
						},
					}
				} else if o.GetName() == "pipelinerun" {
					updated = &v1beta1.PipelineRun{
						TypeMeta: metav1.TypeMeta{
							APIVersion: "tekton.dev/v1beta1",
							Kind:       "PipelineRun",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:       "pipelinerun",
							Namespace:  "test",
							UID:        "pipelinerun-id",
							Generation: 1,
						},
						Status: v1beta1.PipelineRunStatus{
							PipelineRunStatusFields: v1beta1.PipelineRunStatusFields{
								PipelineResults: []v1beta1.PipelineRunResult{
									{
										Name:  "result1",
										Value: "value1-updated",
									},
								},
							},
						},
					}
				} else {
					t.Fatalf("upsertRecord: PipelineRun or TaskRun unsupported %v", o.GetName())
				}
				got, err := client.upsertRecord(ctx, result.GetName(), updated)
				if err != nil {
					t.Fatalf("upsertRecord: %v", err)
				}
				if cmp.Equal(crdToRecord(t, name, o).GetData(), got.GetData(), protocmp.Transform()) {
					t.Errorf(
						"upsertRecord diff (-want, +got):%s\n\n%s",
						crdToRecord(t, name, updated).GetData(), got.GetData(),
					)
				}
			})
		})
	}
}

func TestPut(t *testing.T) {
	ctx := context.Background()
	client := client(t)

	objs := []Object{
		&v1beta1.TaskRun{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "tekton.dev/v1beta1",
				Kind:       "TaskRun",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "taskrun",
				Namespace: "test",
				UID:       "taskrun-id",
			},
		},
		&v1beta1.PipelineRun{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "tekton.dev/v1beta1",
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
		// Run each test 2x - once for the initial creation, another to
		// simulate an update.
		// This is less exhaustive than the other tests, since Put is a wrapper
		// around ensureResult/upsertRecord.
		t.Run(o.GetName(), func(t *testing.T) {
			for _, tc := range []string{"create", "update"} {
				t.Run(tc, func(t *testing.T) {
					if _, _, err := client.Put(ctx, o); err != nil {
						t.Fatal(err)
					}
				})
			}

			// Verify Result/Record exist.
			if _, err := client.GetResult(ctx, &pb.GetResultRequest{
				Name: fmt.Sprintf("test/results/%s", o.GetUID()),
			}); err != nil {
				t.Fatalf("GetResult: %v", err)
			}
			if _, err := client.GetRecord(ctx, &pb.GetRecordRequest{
				Name: fmt.Sprintf("test/results/%s/records/%s", o.GetUID(), o.GetUID()),
			}); err != nil {
				t.Fatalf("GetRecord: %v", err)
			}
		})
	}
}

func crdToRecord(t *testing.T, name string, o Object) *pb.Record {
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
	}
}
