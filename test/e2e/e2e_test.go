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

//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"io/ioutil"
	"testing"

	"github.com/google/go-cmp/cmp"
	resultsv1alpha2 "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"

	"time"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	clientset "github.com/tektoncd/pipeline/pkg/client/clientset/versioned/typed/pipeline/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path"
	"sigs.k8s.io/yaml"
)

const (
	ns = "default"

	defTokenFolder                   = "/tmp/tekton-results/tokens"
	allNamespacesReadAccessTknFile   = "all-namespaces-read-access"
	singleNamespaceReadAccessTknFile = "single-namespace-read-access"
)

var (
	allNamespacesReadAccessPath, singleNamespaceReadAccessPath string
)

func init() {
	tokensFolder := os.Getenv("SA_TOKEN_PATH")
	if len(tokensFolder) == 0 {
		tokensFolder = defTokenFolder
	}
	allNamespacesReadAccessPath = path.Join(tokensFolder, allNamespacesReadAccessTknFile)
	singleNamespaceReadAccessPath = path.Join(tokensFolder, singleNamespaceReadAccessTknFile)
}

func TestTaskRun(t *testing.T) {
	ctx := context.Background()
	tr := new(v1beta1.TaskRun)
	b, err := ioutil.ReadFile("testdata/taskrun.yaml")
	if err != nil {
		t.Fatalf("ioutil.Readfile: %v", err)
	}
	if err := yaml.UnmarshalStrict(b, tr); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	c := client(t)

	// Best effort delete existing Run in case one already exists.
	_ = c.TaskRuns(ns).Delete(ctx, tr.GetName(), metav1.DeleteOptions{})

	tr, err = c.TaskRuns(ns).Create(ctx, tr, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Logf("Created TaskRun %s", tr.GetName())

	// Wait for Result ID to show up.
	t.Run("Result ID", func(t *testing.T) {
		if err := wait.PollImmediate(1*time.Second, 1*time.Minute, func() (done bool, err error) {
			tr, err := c.TaskRuns(ns).Get(ctx, tr.GetName(), metav1.GetOptions{})
			t.Logf("Get: %+v %v", tr.GetName(), err)
			if err != nil {
				return false, nil
			}
			if r, ok := tr.GetAnnotations()["results.tekton.dev/result"]; ok {
				t.Logf("Found Result: %s", r)
				return true, nil
			}
			return false, nil
		}); err != nil {
			t.Fatalf("error waiting for Result ID: %v", err)
		}
	})

	t.Run("Run Cleanup", func(t *testing.T) {
		if err := wait.PollImmediate(1*time.Second, 1*time.Minute, func() (done bool, err error) {
			tr, err := c.TaskRuns(ns).Get(ctx, tr.GetName(), metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return true, nil
			}
			t.Logf("Get: %+v, %v", tr.GetName(), err)
			return false, nil
		}); err != nil {
			t.Fatalf("error waiting TaskRun to be deleted: %v", err)
		}
	})
}

func TestPipelineRun(t *testing.T) {
	ctx := context.Background()
	pr := new(v1beta1.PipelineRun)
	b, err := ioutil.ReadFile("testdata/pipelinerun.yaml")
	if err != nil {
		t.Fatalf("ioutil.Readfile: %v", err)
	}
	if err := yaml.UnmarshalStrict(b, pr); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	c := client(t)

	// Best effort delete existing Run in case one already exists.
	_ = c.PipelineRuns(ns).Delete(ctx, pr.GetName(), metav1.DeleteOptions{})

	if _, err = c.PipelineRuns(ns).Create(ctx, pr, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Wait for Result ID to show up.
	if err := wait.PollImmediate(1*time.Second, 1*time.Minute, func() (done bool, err error) {
		pr, err := c.PipelineRuns(ns).Get(ctx, pr.GetName(), metav1.GetOptions{})
		if err != nil {
			t.Logf("Get: %v", err)
			return false, nil
		}
		if r, ok := pr.GetAnnotations()["results.tekton.dev/result"]; ok {
			t.Logf("Found Result: %s", r)
			return true, nil
		}
		return false, nil
	}); err != nil {
		t.Fatalf("error waiting for Result ID: %v", err)
	}
}

func client(t *testing.T) *clientset.TektonV1beta1Client {
	t.Helper()

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
	config, err := kubeconfig.ClientConfig()
	if err != nil {
		panic(err)
	}
	return clientset.NewForConfigOrDie(config)
}

func TestListResults(t *testing.T) {
	ctx := context.Background()

	t.Run("list results under the default parent", func(t *testing.T) {
		client := newResultsClient(t, allNamespacesReadAccessPath)
		resp, err := client.ListResults(ctx, &resultsv1alpha2.ListResultsRequest{Parent: "default"})
		if err != nil {
			t.Fatal(err)
		}

		if length := len(resp.Results); length == 0 {
			t.Error("No results returned by the API server")
		} else {
			t.Logf("Found %d results\n", length)
		}
	})

	t.Run("list results across parents", func(t *testing.T) {
		client := newResultsClient(t, allNamespacesReadAccessPath)

		// For the purposes of this test suite, listing results under
		// the `default` parent or using the `-` symbol must return the
		// same items. Therefore, let's run both queries and make sure
		// that results are identical.

		want, err := client.ListResults(ctx, &resultsv1alpha2.ListResultsRequest{
			Parent:  "default",
			OrderBy: "created_time",
		})
		if err != nil {
			t.Fatal(err)
		}

		got, err := client.ListResults(ctx, &resultsv1alpha2.ListResultsRequest{
			Parent:  "-",
			OrderBy: "created_time",
		})
		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(want.Results, got.Results, protocmp.Transform()); diff != "" {
			t.Errorf("Mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("return an error because the identity isn't authorized to access all namespaces", func(t *testing.T) {
		client := newResultsClient(t, singleNamespaceReadAccessPath)
		_, err := client.ListResults(ctx, &resultsv1alpha2.ListResultsRequest{Parent: "-"})
		if err == nil {
			t.Fatal("Want an unauthenticated error, but the request succeeded")
		}

		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("API server returned an unexpected error: %v", err)
		}
	})

	t.Run("list results under the default parent using the identity with more limited access", func(t *testing.T) {
		client := newResultsClient(t, singleNamespaceReadAccessPath)
		resp, err := client.ListResults(ctx, &resultsv1alpha2.ListResultsRequest{Parent: "default"})
		if err != nil {
			t.Fatal(err)
		}

		if length := len(resp.Results); length == 0 {
			t.Error("No results returned by the API server")
		} else {
			t.Logf("Found %d results\n", length)
		}
	})
}

func TestListRecords(t *testing.T) {
	ctx := context.Background()

	t.Run("list records by omitting the result name", func(t *testing.T) {
		client := newResultsClient(t, allNamespacesReadAccessPath)
		resp, err := client.ListRecords(ctx, &resultsv1alpha2.ListRecordsRequest{Parent: "default/results/-"})
		if err != nil {
			t.Fatal(err)
		}

		if length := len(resp.Records); length == 0 {
			t.Error("No records returned by the API server")
		} else {
			t.Logf("Found %d records\n", length)
		}
	})

	t.Run("list records by omitting the parent and result names", func(t *testing.T) {
		client := newResultsClient(t, allNamespacesReadAccessPath)

		// For the purposes of this test suite, listing records under
		// the `default/results/-` result or using the `-/results/-`
		// form must return the same items. Therefore, let's run both
		// queries and make sure that results are identical.

		want, err := client.ListRecords(ctx, &resultsv1alpha2.ListRecordsRequest{
			Parent:  "default/results/-",
			OrderBy: "created_time",
		})
		if err != nil {
			t.Fatal(err)
		}

		got, err := client.ListRecords(ctx, &resultsv1alpha2.ListRecordsRequest{
			Parent:  "-/results/-",
			OrderBy: "created_time",
		})
		if err != nil {
			t.Fatal(err)
		}

		// Compare only record names. Comparing records data is susceptable to race conditions.
		if diff := cmp.Diff(recordNames(t, want.Records), recordNames(t, got.Records), protocmp.Transform()); diff != "" {
			t.Errorf("Mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("return an error because the identity isn't authorized to access all namespaces", func(t *testing.T) {
		client := newResultsClient(t, singleNamespaceReadAccessPath)
		_, err := client.ListRecords(ctx, &resultsv1alpha2.ListRecordsRequest{Parent: "-/results/-"})
		if err == nil {
			t.Fatal("Want an unauthenticated error, but the request succeeded")
		}

		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("API server returned an unexpected error: %v", err)
		}
	})

	t.Run("list records using the identity with more limited access", func(t *testing.T) {
		client := newResultsClient(t, singleNamespaceReadAccessPath)
		resp, err := client.ListRecords(ctx, &resultsv1alpha2.ListRecordsRequest{Parent: "default/results/-"})
		if err != nil {
			t.Fatal(err)
		}

		if length := len(resp.Records); length == 0 {
			t.Error("No records returned by the API server")
		} else {
			t.Logf("Found %d records\n", length)
		}
	})
}

func recordNames(t *testing.T, records []*resultsv1alpha2.Record) []string {
	t.Helper()

	ret := make([]string, len(records))
	for _, record := range records {
		ret = append(ret, record.GetName())
	}
	return ret
}
