// Copyright 2026 The Tekton Authors
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

package namespace

import (
	"context"
	"fmt"
	"testing"
	"time"

	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	corev1listers "k8s.io/client-go/listers/core/v1"
)

type mockResultsClient struct {
	pb.ResultsClient
	results       map[string]*pb.Result
	deletedNames  []string
	listErr       error
	deleteErr     error
	deleteErrName string
}

func (m *mockResultsClient) ListResults(_ context.Context, req *pb.ListResultsRequest, _ ...grpc.CallOption) (*pb.ListResultsResponse, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var results []*pb.Result
	for _, r := range m.results {
		if req.GetParent() == "" || getParent(r.GetName()) == req.GetParent() {
			results = append(results, r)
		}
	}
	return &pb.ListResultsResponse{Results: results}, nil
}

func (m *mockResultsClient) DeleteResult(_ context.Context, req *pb.DeleteResultRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	if m.deleteErr != nil && req.GetName() == m.deleteErrName {
		return nil, m.deleteErr
	}
	m.deletedNames = append(m.deletedNames, req.GetName())
	delete(m.results, req.GetName())
	return &emptypb.Empty{}, nil
}

func getParent(name string) string {
	for i, c := range name {
		if c == '/' {
			return name[:i]
		}
	}
	return name
}

type mockNamespaceLister struct {
	namespaces map[string]*corev1.Namespace
}

func (m *mockNamespaceLister) List(_ labels.Selector) ([]*corev1.Namespace, error) {
	var result []*corev1.Namespace
	for _, ns := range m.namespaces {
		result = append(result, ns)
	}
	return result, nil
}

func (m *mockNamespaceLister) Get(name string) (*corev1.Namespace, error) {
	ns, ok := m.namespaces[name]
	if !ok {
		return nil, fmt.Errorf("namespace %q not found: %w", name, &notFoundError{})
	}
	return ns, nil
}

type notFoundError struct{}

func (e *notFoundError) Error() string { return "not found" }
func (e *notFoundError) Status() metav1.Status {
	return metav1.Status{Reason: metav1.StatusReasonNotFound}
}

var _ corev1listers.NamespaceLister = &mockNamespaceLister{}

func TestReconcile_ActiveNamespace_NoOp(t *testing.T) {
	client := &mockResultsClient{
		results: map[string]*pb.Result{
			"active-ns/results/abc": {Name: "active-ns/results/abc"},
		},
	}
	lister := &mockNamespaceLister{
		namespaces: map[string]*corev1.Namespace{
			"active-ns": {ObjectMeta: metav1.ObjectMeta{Name: "active-ns"}},
		},
	}
	r := &Reconciler{resultsClient: client, namespaceLister: lister}

	err := r.Reconcile(context.Background(), "active-ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.deletedNames) != 0 {
		t.Errorf("expected no deletions, got %v", client.deletedNames)
	}
}

func TestReconcile_DeletedNamespace_CleansUpResults(t *testing.T) {
	client := &mockResultsClient{
		results: map[string]*pb.Result{
			"deleted-ns/results/aaa": {Name: "deleted-ns/results/aaa"},
			"deleted-ns/results/bbb": {Name: "deleted-ns/results/bbb"},
			"other-ns/results/ccc":   {Name: "other-ns/results/ccc"},
		},
	}
	lister := &mockNamespaceLister{namespaces: map[string]*corev1.Namespace{}}
	r := &Reconciler{resultsClient: client, namespaceLister: lister}

	err := r.Reconcile(context.Background(), "deleted-ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.deletedNames) != 2 {
		t.Errorf("expected 2 deletions, got %d: %v", len(client.deletedNames), client.deletedNames)
	}
	if _, exists := client.results["other-ns/results/ccc"]; !exists {
		t.Error("result from other namespace should not be deleted")
	}
}

func TestReconcile_TerminatingNamespace_CleansUpResults(t *testing.T) {
	now := metav1.NewTime(time.Now())
	client := &mockResultsClient{
		results: map[string]*pb.Result{
			"terminating-ns/results/aaa": {Name: "terminating-ns/results/aaa"},
		},
	}
	lister := &mockNamespaceLister{
		namespaces: map[string]*corev1.Namespace{
			"terminating-ns": {
				ObjectMeta: metav1.ObjectMeta{
					Name:              "terminating-ns",
					DeletionTimestamp: &now,
				},
			},
		},
	}
	r := &Reconciler{resultsClient: client, namespaceLister: lister}

	err := r.Reconcile(context.Background(), "terminating-ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.deletedNames) != 1 {
		t.Errorf("expected 1 deletion, got %d", len(client.deletedNames))
	}
}

func TestReconcile_ListResultsError_ReturnsError(t *testing.T) {
	client := &mockResultsClient{
		results: map[string]*pb.Result{},
		listErr: fmt.Errorf("connection refused"),
	}
	lister := &mockNamespaceLister{namespaces: map[string]*corev1.Namespace{}}
	r := &Reconciler{resultsClient: client, namespaceLister: lister}

	err := r.Reconcile(context.Background(), "deleted-ns")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestReconcile_DeleteResultPartialFailure_Continues(t *testing.T) {
	client := &mockResultsClient{
		results: map[string]*pb.Result{
			"deleted-ns/results/aaa": {Name: "deleted-ns/results/aaa"},
			"deleted-ns/results/bbb": {Name: "deleted-ns/results/bbb"},
		},
		deleteErr:     fmt.Errorf("permission denied"),
		deleteErrName: "deleted-ns/results/aaa",
	}
	lister := &mockNamespaceLister{namespaces: map[string]*corev1.Namespace{}}
	r := &Reconciler{resultsClient: client, namespaceLister: lister}

	err := r.Reconcile(context.Background(), "deleted-ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.deletedNames) != 1 {
		t.Errorf("expected 1 successful deletion, got %d", len(client.deletedNames))
	}
}

func TestReconcile_EmptyResults_CompletesSuccessfully(t *testing.T) {
	client := &mockResultsClient{results: map[string]*pb.Result{}}
	lister := &mockNamespaceLister{namespaces: map[string]*corev1.Namespace{}}
	r := &Reconciler{resultsClient: client, namespaceLister: lister}

	err := r.Reconcile(context.Background(), "deleted-ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.deletedNames) != 0 {
		t.Errorf("expected no deletions, got %v", client.deletedNames)
	}
}
