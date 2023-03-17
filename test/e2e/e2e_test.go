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
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/results/test/e2e/client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/transport"

	resultsv1alpha2 "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"
	"knative.dev/pkg/apis"

	"time"

	"os"
	"path"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	tektonv1beta1client "github.com/tektoncd/pipeline/pkg/client/clientset/versioned/typed/pipeline/v1beta1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"
)

const (
	defaultServerName                           = "tekton-results-api-service.tekton-pipelines.svc.cluster.local"
	defaultServerAddress                        = "https://localhost:8080"
	defaultCertFileName                         = "tekton-results-cert.pem"
	allNamespacesReadAccessTokenFileName        = "all-namespaces-read-access"
	singleNamespaceReadAccessTokenFileName      = "single-namespace-read-access"
	allNamespacesAdminAccessTokenFileName       = "all-namespaces-admin-access"
	allNamespacesImpersonateAccessTokenFileName = "all-namespaces-impersonate-access"
	defaultCertPath                             = "/tmp/tekton-results/ssl"
	defaultTokenPath                            = "/tmp/tekton-results/tokens"
	defaultNamespace                            = "default"
)

var (
	allNamespacesReadAccessTokenFile,
	singleNamespaceReadAccessTokenFile,
	allNamespacesAdminAccessTokenFile,
	allNamespacesImpersonateAccessTokenFile,
	certFile string
	serverName    string
	serverAddress string
)

func init() {
	certPath := os.Getenv("SSL_CERT_PATH")
	if len(certPath) == 0 {
		certPath = defaultCertPath
	}

	certFileName := os.Getenv("CERT_FILE_NAME")
	if len(certFileName) == 0 {
		certFileName = defaultCertFileName
	}
	certFile = path.Join(certPath, certFileName)

	tokenPath := os.Getenv("SA_TOKEN_PATH")
	if len(tokenPath) == 0 {
		tokenPath = defaultTokenPath
	}

	apiServerName := os.Getenv("API_SERVER_NAME")
	if len(apiServerName) == 0 {
		apiServerName = defaultServerName
	}
	serverName = apiServerName

	apiServerAddress := os.Getenv("API_SERVER_ADDR")
	if len(apiServerAddress) == 0 {
		apiServerAddress = defaultServerAddress
	}
	serverAddress = apiServerAddress

	allNamespacesReadAccessTokenFile = path.Join(tokenPath, allNamespacesReadAccessTokenFileName)
	singleNamespaceReadAccessTokenFile = path.Join(tokenPath, singleNamespaceReadAccessTokenFileName)
	allNamespacesAdminAccessTokenFile = path.Join(tokenPath, allNamespacesAdminAccessTokenFileName)
	allNamespacesImpersonateAccessTokenFile = path.Join(tokenPath, allNamespacesImpersonateAccessTokenFileName)
}

func TestTaskRun(t *testing.T) {
	ctx := context.Background()
	tr := new(tektonv1beta1.TaskRun)
	b, err := os.ReadFile("testdata/taskrun.yaml")
	if err != nil {
		t.Fatalf("Error reading file: %v", err)
	}
	if err := yaml.UnmarshalStrict(b, tr); err != nil {
		t.Fatalf("Erro unmarshalling: %v", err)
	}

	tc := tektonClient(t)

	// Best effort delete existing Run in case one already exists.
	_ = tc.TaskRuns(defaultNamespace).Delete(ctx, tr.GetName(), metav1.DeleteOptions{})

	tr, err = tc.TaskRuns(defaultNamespace).Create(ctx, tr, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Error creating TaskRun: %v", err)
	}

	gc, _ := resultsClient(t, allNamespacesReadAccessTokenFile, nil)

	var resName, recName string

	// Wait for Result ID to show up.
	t.Run("check annotations", func(t *testing.T) {
		if err := wait.PollImmediate(1*time.Second, 1*time.Minute, func() (done bool, err error) {
			tr, err := tc.TaskRuns(defaultNamespace).Get(ctx, tr.GetName(), metav1.GetOptions{})
			if err != nil {
				t.Fatalf("Error getting TaskRun: %v", err)
			}
			var resAnnotation, recAnnotation bool
			resName, resAnnotation = tr.GetAnnotations()["results.tekton.dev/result"]
			recName, recAnnotation = tr.GetAnnotations()["results.tekton.dev/record"]
			if resAnnotation && recAnnotation {
				return true, nil
			}
			return false, nil
		}); err != nil {
			t.Fatalf("error waiting for Result ID: %v", err)
		}
	})

	t.Run("check deletion", func(t *testing.T) {
		if err := wait.PollImmediate(1*time.Second, 1*time.Minute, func() (done bool, err error) {
			_, err = tc.TaskRuns(defaultNamespace).Get(ctx, tr.GetName(), metav1.GetOptions{})
			if err != nil {
				if k8serrors.IsNotFound(err) {
					return true, nil
				}
				t.Errorf("Error getting PipelineRun: %v", err)
				return false, err
			}
			return false, nil
		}); err != nil {
			t.Fatalf("Error waiting for TaskRun deletion: %v", err)
		}
	})

	t.Run("check result", func(t *testing.T) {
		if resName == "" {
			t.Skip("Result name not found")
		}
		_, err = gc.GetResult(context.Background(), &resultsv1alpha2.GetResultRequest{Name: resName})
		if err != nil {
			t.Errorf("Error getting Result: %v", err)
		}
	})

	t.Run("check record", func(t *testing.T) {
		if recName == "" {
			t.Skip("Record name not found")
		}
		_, err = gc.GetRecord(context.Background(), &resultsv1alpha2.GetRecordRequest{Name: recName})
		if err != nil {
			t.Errorf("Error getting Record: %v", err)
		}
	})
}

func TestPipelineRun(t *testing.T) {
	ctx := context.Background()
	pr := new(tektonv1beta1.PipelineRun)
	b, err := os.ReadFile("testdata/pipelinerun.yaml")
	if err != nil {
		t.Fatalf("Error reading file: %v", err)
	}
	if err := yaml.UnmarshalStrict(b, pr); err != nil {
		t.Fatalf("Error unmarshalling: %v", err)
	}

	tc := tektonClient(t)

	// Best effort delete existing Run in case one already exists.
	_ = tc.PipelineRuns(defaultNamespace).Delete(ctx, pr.GetName(), metav1.DeleteOptions{})

	if _, err = tc.PipelineRuns(defaultNamespace).Create(ctx, pr, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Error creating PipelineRun: %v", err)
	}

	gc, _ := resultsClient(t, allNamespacesReadAccessTokenFile, nil)

	var resName, recName string

	t.Run("check annotations", func(t *testing.T) {
		// Wait for Result ID to show up.
		if err := wait.PollImmediate(1*time.Second, 1*time.Minute, func() (done bool, err error) {
			pr, err := tc.PipelineRuns(defaultNamespace).Get(ctx, pr.GetName(), metav1.GetOptions{})
			if err != nil {
				t.Fatalf("Error getting PipelineRun: %v", err)
			}
			var resAnnotation, recAnnotation bool
			resName, resAnnotation = pr.GetAnnotations()["results.tekton.dev/result"]
			recName, recAnnotation = pr.GetAnnotations()["results.tekton.dev/record"]
			if resAnnotation && recAnnotation {
				return true, nil
			}
			return false, nil
		}); err != nil {
			t.Fatalf("Error waiting for PipelineRun creation: %v", err)
		}
	})

	t.Run("check deletion", func(t *testing.T) {
		if err := wait.PollImmediate(1*time.Second, 1*time.Minute, func() (done bool, err error) {
			_, err = tc.PipelineRuns(defaultNamespace).Get(ctx, pr.GetName(), metav1.GetOptions{})
			if err != nil {
				if k8serrors.IsNotFound(err) {
					return true, nil
				}
				t.Errorf("Error getting PipelineRun: %v", err)
				return false, err
			}
			return false, nil
		}); err != nil {
			t.Fatalf("Error waiting for PipelineRun deletion: %v", err)
		}
	})

	t.Run("check result", func(t *testing.T) {
		if resName == "" {
			t.Skip("Result name not found")
		}
		_, err := gc.GetResult(context.Background(), &resultsv1alpha2.GetResultRequest{Name: resName})
		if err != nil {
			t.Fatalf("Error getting Result: %v", err)
		}
	})

	t.Run("check record", func(t *testing.T) {
		if recName == "" {
			t.Skip("Record name not found")
		}
		_, err = gc.GetRecord(context.Background(), &resultsv1alpha2.GetRecordRequest{Name: recName})
		if err != nil {
			t.Errorf("Error getting Record: %v", err)
		}
	})

	t.Run("result data consistency", func(t *testing.T) {
		result, err := gc.GetResult(context.Background(), &resultsv1alpha2.GetResultRequest{
			Name: resName,
		})
		if err != nil {
			t.Fatal(err)
		}

		t.Run("Result and RecordSummary Annotations were set accordingly", func(t *testing.T) {
			if diff := cmp.Diff(map[string]string{
				"repo":   "tektoncd/results",
				"commit": "1a6b908",
			}, result.Annotations); diff != "" {
				t.Errorf("Result.Annotations: mismatch (-want +got):\n%s", diff)
			}

			if diff := cmp.Diff(map[string]string{
				"foo": "bar",
			}, result.Summary.Annotations); diff != "" {
				t.Errorf("Result.Summary.Annotations: mismatch (-want +got):\n%s", diff)
			}
		})

		t.Run("the PipelineRun was archived in its final state", func(t *testing.T) {
			wantStatus := resultsv1alpha2.RecordSummary_SUCCESS
			gotStatus := result.Summary.Status
			if wantStatus != gotStatus {
				t.Fatalf("Result.Summary.Status: want %v, but got %v", wantStatus, gotStatus)
			}

			record, err := gc.GetRecord(context.Background(), &resultsv1alpha2.GetRecordRequest{
				Name: result.Summary.Record,
			})
			if err != nil {
				t.Fatal(err)
			}

			var pipelineRun v1beta1.PipelineRun
			if err := json.Unmarshal(record.Data.Value, &pipelineRun); err != nil {
				t.Fatal(err)
			}

			if !pipelineRun.IsDone() {
				t.Fatal("Want PipelineRun to be done, but it isn't")
			}

			wantReason := v1beta1.PipelineRunReasonSuccessful
			if gotReason := pipelineRun.Status.GetCondition(apis.ConditionSucceeded).GetReason(); wantReason != v1beta1.PipelineRunReason(gotReason) {
				t.Fatalf("PipelineRun: want condition reason %s, but got %s", wantReason, gotReason)
			}
		})
	})
}

func clientConfig(t *testing.T) *rest.Config {
	t.Helper()

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
	config, err := clientConfig.ClientConfig()
	if err != nil {
		t.Fatalf("Error creating client config: %v", err)
	}
	return config
}

func tektonClient(t *testing.T) *tektonv1beta1client.TektonV1beta1Client {
	t.Helper()

	return tektonv1beta1client.NewForConfigOrDie(clientConfig(t))
}

func resultsClient(t *testing.T, tokenFile string, impersonationConfig *transport.ImpersonationConfig) (client.GRPCClient, client.RESTClient) {
	t.Helper()

	if impersonationConfig == nil {
		impersonationConfig = &transport.ImpersonationConfig{}
	}

	transportCredentials, err := credentials.NewClientTLSFromFile(certFile, serverName)
	if err != nil {
		t.Fatalf("Error creating client TLS: %v", err)
	}

	callOptions := []grpc.CallOption{
		grpc.PerRPCCredentials(&client.CustomCredentials{
			TokenSource:         transport.NewCachedFileTokenSource(tokenFile),
			ImpersonationConfig: impersonationConfig,
		}),
	}

	grpcOptions := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithDefaultCallOptions(callOptions...),
		grpc.WithTransportCredentials(transportCredentials),
	}

	grpcClient, err := client.NewGRPCClient(serverAddress, grpcOptions...)
	if err != nil {
		t.Fatalf("Error creating gRPC client: %v", err)
	}

	restConfig := &transport.Config{
		TLS: transport.TLSConfig{
			CAFile:     certFile,
			ServerName: serverName,
		},
		BearerTokenFile: tokenFile,
		Impersonate:     *impersonationConfig,
	}

	restOptions := []client.RestOption{
		client.WithConfig(restConfig),
	}

	restClient, err := client.NewRESTClient(serverAddress, restOptions...)
	if err != nil {
		t.Fatalf("Error creating REST request: %v", err)
	}

	return grpcClient, restClient
}

func TestGRPCLogging(t *testing.T) {
	ctx := context.Background()

	// ignore old logs
	sinceTime := metav1.Now()
	podLogOptions := corev1.PodLogOptions{
		SinceTime: &sinceTime,
	}

	matcher := "\"grpc.method\":\"ListResults\""

	gc, _ := resultsClient(t, allNamespacesReadAccessTokenFile, nil)

	t.Run("log entry is found when not expected", func(t *testing.T) {
		resultsApiLogs, err := getResultsApiLogs(ctx, &podLogOptions, t)
		if err != nil {
			t.Fatal(err)
		}

		if strings.Contains(resultsApiLogs, matcher) {
			t.Errorf("Found log match for %s in logs %s when there should not be", matcher, resultsApiLogs)
		}
	})

	t.Run("log entry is found when expected", func(t *testing.T) {
		_, err := gc.ListResults(ctx, &resultsv1alpha2.ListResultsRequest{Parent: "default"})
		if err != nil {
			t.Fatal(err)
		}

		resultsApiLogs, err := getResultsApiLogs(ctx, &podLogOptions, t)
		if err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(resultsApiLogs, matcher) {
			t.Errorf("No match for %s in logs %s", matcher, resultsApiLogs)
		}
	})
}

// Returns a string of api pods logs concatenated
func getResultsApiLogs(ctx context.Context, podLogOptions *corev1.PodLogOptions, t *testing.T) (string, error) {
	t.Helper()
	const apiPodBasename = "tekton-results-api"
	const nsResults = "tekton-pipelines"

	clientset := kubernetes.NewForConfigOrDie(clientConfig(t))

	pods, err := clientset.CoreV1().Pods(nsResults).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", err
	}

	numApiPods := 0
	var apiPodsLogs []string
	for _, pod := range pods.Items {
		// find api pods
		if strings.HasPrefix(pod.Name, apiPodBasename) {
			numApiPods++
			// read pod logs
			podLogRequest := clientset.CoreV1().Pods(nsResults).GetLogs(pod.Name, podLogOptions)
			stream, err := podLogRequest.Stream(ctx)
			if err != nil {
				return "", err
			}
			defer stream.Close()
			podLogBytes, err := io.ReadAll(stream)
			if err != nil {
				return "", err
			}
			apiPodsLogs = append(apiPodsLogs, string(podLogBytes))
		}
	}

	if numApiPods == 0 {
		return "", errors.New("no " + apiPodBasename + "pod found")
	}

	return strings.Join(apiPodsLogs, ""), nil
}

func TestListResults(t *testing.T) {
	ctx := context.Background()

	t.Run("list results under the default parent", func(t *testing.T) {
		gc, _ := resultsClient(t, allNamespacesReadAccessTokenFile, nil)

		res, err := gc.ListResults(ctx, &resultsv1alpha2.ListResultsRequest{Parent: "default"})
		if err != nil {
			t.Fatalf("Error listing Results: %v", err)
		}

		if length := len(res.Results); length == 0 {
			t.Error("No Results returned by the API server")
		}
	})

	t.Run("list results across parents", func(t *testing.T) {
		gc, _ := resultsClient(t, allNamespacesReadAccessTokenFile, nil)

		// For the purposes of this test suite, listing results under
		// the `default` parent or using the `-` symbol must return the
		// same items. Therefore, let's run both queries and make sure
		// that results are identical.

		want, err := gc.ListResults(ctx, &resultsv1alpha2.ListResultsRequest{
			Parent:  "default",
			OrderBy: "create_time",
		})
		if err != nil {
			t.Fatal(err)
		}

		got, err := gc.ListResults(ctx, &resultsv1alpha2.ListResultsRequest{
			Parent:  "-",
			OrderBy: "create_time",
		})
		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(want.Results, got.Results, protocmp.Transform()); diff != "" {
			t.Errorf("Mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("return an error because the identity isn't authorized to access all namespaces", func(t *testing.T) {
		gc, _ := resultsClient(t, singleNamespaceReadAccessTokenFile, nil)
		_, err := gc.ListResults(ctx, &resultsv1alpha2.ListResultsRequest{Parent: "-"})
		if err == nil {
			t.Fatal("Want an unauthenticated error, but the request succeeded")
		}

		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("API server returned an unexpected error: %v", err)
		}
	})

	t.Run("list results under the default parent using the identity with more limited access", func(t *testing.T) {
		gc, _ := resultsClient(t, singleNamespaceReadAccessTokenFile, nil)
		res, err := gc.ListResults(ctx, &resultsv1alpha2.ListResultsRequest{Parent: "default"})
		if err != nil {
			t.Fatal(err)
		}

		if length := len(res.Results); length == 0 {
			t.Error("No Results returned by the API server")
		}
	})

	t.Run("grpc and rest consistency", func(t *testing.T) {
		parent := "default"
		gc, rc := resultsClient(t, allNamespacesReadAccessTokenFile, nil)
		want, err := gc.ListResults(ctx, &resultsv1alpha2.ListResultsRequest{Parent: parent})
		if err != nil {
			t.Fatalf("Error listing Results: %v", err)
		}

		got, err := rc.ListResults(ctx, &resultsv1alpha2.ListResultsRequest{Parent: parent})
		if err != nil {
			t.Fatalf("Error listing Results: %v", err)
		}

		if diff := cmp.Diff(want.Results, got.Results, protocmp.Transform()); diff != "" {
			t.Errorf("Mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestListRecords(t *testing.T) {
	ctx := context.Background()

	t.Run("list records by omitting the result name", func(t *testing.T) {
		gc, _ := resultsClient(t, allNamespacesReadAccessTokenFile, nil)
		res, err := gc.ListRecords(ctx, &resultsv1alpha2.ListRecordsRequest{Parent: "default/results/-"})
		if err != nil {
			t.Fatal(err)
		}

		if length := len(res.Records); length == 0 {
			t.Error("No Records returned by the API server")
		}
	})

	t.Run("list records by omitting the parent and result names", func(t *testing.T) {
		gc, _ := resultsClient(t, allNamespacesReadAccessTokenFile, nil)

		// For the purposes of this test suite, listing records under
		// the `default/results/-` result or using the `-/results/-`
		// form must return the same items. Therefore, let's run both
		// queries and make sure that results are identical.

		want, err := gc.ListRecords(ctx, &resultsv1alpha2.ListRecordsRequest{
			Parent:  "default/results/-",
			OrderBy: "create_time",
		})
		if err != nil {
			t.Fatal(err)
		}

		got, err := gc.ListRecords(ctx, &resultsv1alpha2.ListRecordsRequest{
			Parent:  "-/results/-",
			OrderBy: "create_time",
		})
		if err != nil {
			t.Fatal(err)
		}

		// Compare only record names. Comparing records data is susceptible to race conditions.
		if diff := cmp.Diff(recordNames(t, want.Records), recordNames(t, got.Records), protocmp.Transform()); diff != "" {
			t.Errorf("Mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("return an error because the identity isn't authorized to access all namespaces", func(t *testing.T) {
		gc, _ := resultsClient(t, singleNamespaceReadAccessTokenFile, nil)
		_, err := gc.ListRecords(ctx, &resultsv1alpha2.ListRecordsRequest{Parent: "-/results/-"})
		if err == nil {
			t.Fatal("Want an unauthenticated error, but the request succeeded")
		}
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("API server returned an unexpected error: %v", err)
		}
	})

	t.Run("list records using the identity with more limited access", func(t *testing.T) {
		gc, _ := resultsClient(t, singleNamespaceReadAccessTokenFile, nil)
		resp, err := gc.ListRecords(ctx, &resultsv1alpha2.ListRecordsRequest{Parent: "default/results/-"})
		if err != nil {
			t.Fatal(err)
		}
		if length := len(resp.Records); length == 0 {
			t.Error("No Records returned by the API server")
		}
	})

	t.Run("grpc and rest consistency", func(t *testing.T) {
		parent := "default/results/-"
		gc, rc := resultsClient(t, allNamespacesReadAccessTokenFile, nil)
		want, err := gc.ListRecords(ctx, &resultsv1alpha2.ListRecordsRequest{Parent: parent})
		if err != nil {
			t.Fatalf("Error listing Records: %v", err)
		}

		got, err := rc.ListRecords(ctx, &resultsv1alpha2.ListRecordsRequest{Parent: parent})
		if err != nil {
			t.Fatalf("Error listing Records: %v", err)
		}

		if diff := cmp.Diff(want.Records, got.Records, protocmp.Transform()); diff != "" {
			t.Errorf("Mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestGetResult(t *testing.T) {
	ctx := context.Background()
	gc, rc := resultsClient(t, allNamespacesReadAccessTokenFile, nil)

	list, err := gc.ListResults(ctx, &resultsv1alpha2.ListResultsRequest{Parent: "default"})
	if err != nil {
		t.Fatalf("Error listing Results: %v", err)
	}

	name := list.Results[0].GetName()
	want, got := &resultsv1alpha2.Result{}, &resultsv1alpha2.Result{}

	t.Run("get result", func(t *testing.T) {
		t.Run("grpc", func(t *testing.T) {
			want, err = gc.GetResult(ctx, &resultsv1alpha2.GetResultRequest{Name: name})
			if err != nil {
				t.Fatalf("Error getting Result: %v", err)
			}
		})
		t.Run("rest", func(t *testing.T) {
			got, err = rc.GetResult(ctx, &resultsv1alpha2.GetResultRequest{Name: name})
			if err != nil {
				t.Fatalf("Error getting Result: %v", err)
			}
		})
	})

	t.Run("grpc and rest consistency", func(t *testing.T) {
		if err != nil {
			t.Skip("Required tests failed")
		}
		if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
			t.Errorf("Mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestGetRecord(t *testing.T) {
	ctx := context.Background()
	gc, rc := resultsClient(t, allNamespacesReadAccessTokenFile, nil)

	list, err := gc.ListRecords(ctx, &resultsv1alpha2.ListRecordsRequest{Parent: "default/results/-"})
	if err != nil {
		t.Fatalf("Error listing Records: %v", err)
	}

	name := list.Records[0].GetName()
	want, got := &resultsv1alpha2.Record{}, &resultsv1alpha2.Record{}

	t.Run("get record", func(t *testing.T) {
		t.Run("grpc", func(t *testing.T) {
			want, err = gc.GetRecord(ctx, &resultsv1alpha2.GetRecordRequest{Name: name})
			if err != nil {
				t.Fatalf("Error getting Record: %v", err)
			}
		})

		t.Run("rest", func(t *testing.T) {
			got, err = rc.GetRecord(ctx, &resultsv1alpha2.GetRecordRequest{Name: name})
			if err != nil {
				t.Fatalf("Error getting Record: %v", err)
			}
		})
	})

	t.Run("grpc and rest consistency", func(t *testing.T) {
		if err != nil {
			t.Skip("Required tests failed")
		}
		if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
			t.Errorf("Mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestDeleteRecord(t *testing.T) {
	ctx := context.Background()
	gc, rc := resultsClient(t, allNamespacesAdminAccessTokenFile, nil)

	list, err := gc.ListRecords(ctx, &resultsv1alpha2.ListRecordsRequest{Parent: "default/results/-"})
	if err != nil {
		t.Fatalf("Error listing Records: %v", err)
	}

	t.Run("delete record", func(t *testing.T) {
		t.Run("grpc", func(t *testing.T) {
			_, err := gc.DeleteRecord(ctx, &resultsv1alpha2.DeleteRecordRequest{Name: list.Records[0].GetName()})
			if err != nil {
				t.Fatalf("Error deleting Record: %v", err)
			}
			_, err = gc.GetRecord(ctx, &resultsv1alpha2.GetRecordRequest{Name: list.Records[0].GetName()})
			if err == nil {
				t.Fatalf("Expected error, but no error found")
			} else if status.Code(err) != codes.NotFound {
				t.Fatalf("Error getting Record: %v", err)
			}
		})

		t.Run("rest", func(t *testing.T) {
			_, err := rc.DeleteRecord(ctx, &resultsv1alpha2.DeleteRecordRequest{Name: list.Records[1].GetName()})
			if err != nil {
				t.Fatalf("Error deleting Record: %v", err)
			}
			_, err = rc.GetRecord(ctx, &resultsv1alpha2.GetRecordRequest{Name: list.Records[1].GetName()})
			if err == nil {
				t.Fatalf("Expected error, but no error found")
			} else if err.Error() != http.StatusText(http.StatusNotFound) {
				t.Fatalf("Error getting Record: %v", err)
			}
		})
	})
}

func TestDeleteResult(t *testing.T) {
	ctx := context.Background()
	gc, rc := resultsClient(t, allNamespacesAdminAccessTokenFile, nil)

	list, err := gc.ListResults(ctx, &resultsv1alpha2.ListResultsRequest{Parent: "default"})
	if err != nil {
		t.Fatalf("Error listing Results: %v", err)
	}

	t.Run("delete result", func(t *testing.T) {
		t.Run("grpc", func(t *testing.T) {
			_, err := gc.DeleteResult(ctx, &resultsv1alpha2.DeleteResultRequest{Name: list.Results[0].GetName()})
			if err != nil {
				t.Fatalf("Error deleting Result: %v", err)
			}
			_, err = gc.GetResult(ctx, &resultsv1alpha2.GetResultRequest{Name: list.Results[0].GetName()})
			if err == nil {
				t.Fatalf("Expected error, but no error found")
			} else if status.Code(err) != codes.NotFound {
				t.Fatalf("Error getting Result: %v", err)
			}
		})

		t.Run("rest", func(t *testing.T) {
			_, err := rc.DeleteResult(ctx, &resultsv1alpha2.DeleteResultRequest{Name: list.Results[1].GetName()})
			if err != nil {
				t.Fatalf("Error deleting Result: %v", err)
			}
			_, err = rc.GetResult(ctx, &resultsv1alpha2.GetResultRequest{Name: list.Results[1].GetName()})
			if err == nil {
				t.Fatalf("Expected error, but no error found")
			} else if err.Error() != http.StatusText(http.StatusNotFound) {
				t.Fatalf("Error getting Result: %v", err)
			}
		})
	})
}

func TestAuthentication(t *testing.T) {
	ctx := context.Background()
	invalidTokenFile := path.Join(defaultTokenPath, "invalid-token")
	err := os.WriteFile(invalidTokenFile, []byte("invalid token"), 0666)
	if err != nil {
		t.Fatalf("Error writing file: %v", err)
	}
	p := "default"

	t.Run("valid token", func(t *testing.T) {
		gc, rc := resultsClient(t, allNamespacesReadAccessTokenFile, nil)
		t.Run("grpc", func(t *testing.T) {
			_, err = gc.ListResults(ctx, &resultsv1alpha2.ListResultsRequest{Parent: p})
			if err != nil {
				t.Fatalf("Error listing Results: %v", err)
			}
		})
		t.Run("rest", func(t *testing.T) {
			_, err = rc.ListResults(ctx, &resultsv1alpha2.ListResultsRequest{Parent: p})
			if err != nil {
				t.Fatalf("Error listing Results: %v", err)
			}
		})
	})

	t.Run("invalid token", func(t *testing.T) {
		gc, rc := resultsClient(t, invalidTokenFile, nil)
		t.Run("grpc", func(t *testing.T) {
			_, err = gc.ListResults(ctx, &resultsv1alpha2.ListResultsRequest{Parent: p})
			if err == nil {
				t.Fatalf("Expected error, but no error found")
			} else if status.Code(err) != codes.Unauthenticated {
				t.Fatalf("Error listing Results: %v", err)
			}
		})
		t.Run("rest", func(t *testing.T) {
			_, err = rc.ListResults(ctx, &resultsv1alpha2.ListResultsRequest{Parent: p})
			if err == nil {
				t.Fatalf("Expected error, but no error found")
			} else if err.Error() != http.StatusText(http.StatusUnauthorized) {
				t.Fatalf("Error listing Results: %v", err)
			}
		})
	})
}

func TestAuthorization(t *testing.T) {
	ctx := context.Background()
	gc, rc := resultsClient(t, singleNamespaceReadAccessTokenFile, nil)

	t.Run("unauthorized token", func(t *testing.T) {
		p := "tekton"
		t.Run("grpc", func(t *testing.T) {
			_, err := gc.ListResults(ctx, &resultsv1alpha2.ListResultsRequest{Parent: p})
			if err == nil {
				t.Fatalf("Expected error, but no error found")
			} else if status.Code(err) != codes.Unauthenticated {
				t.Fatalf("Error listing Results: %v", err)
			}
		})
		t.Run("rest", func(t *testing.T) {
			_, err := rc.ListResults(ctx, &resultsv1alpha2.ListResultsRequest{Parent: p})
			if err == nil {
				t.Fatalf("Expected error, but no error found")
			} else if err.Error() != http.StatusText(http.StatusUnauthorized) {
				t.Fatalf("Error listing Results: %v", err)
			}
		})
	})
}

func TestImpersonation(t *testing.T) {
	ctx := context.Background()
	p := "default"
	t.Run("impersonate with user not having permission", func(t *testing.T) {
		gc, rc := resultsClient(t, allNamespacesImpersonateAccessTokenFile, &transport.ImpersonationConfig{
			UserName: "system:serviceaccount:default:default",
		})
		t.Run("grpc", func(t *testing.T) {
			_, err := gc.ListResults(ctx, &resultsv1alpha2.ListResultsRequest{Parent: p})
			if err == nil {
				t.Fatalf("Expected error, but no error found")
			} else if status.Code(err) != codes.Unauthenticated {
				t.Fatalf("Error listing Results: %v", err)
			}
		})
		t.Run("rest", func(t *testing.T) {
			_, err := rc.ListResults(ctx, &resultsv1alpha2.ListResultsRequest{Parent: p})
			if err == nil {
				t.Fatalf("Expected error, but no error found")
			} else if err.Error() != http.StatusText(http.StatusUnauthorized) {
				t.Fatalf("Error listing Results: %v", err)
			}
		})
	})

	t.Run("impersonate with user having permission", func(t *testing.T) {
		gc, rc := resultsClient(t, allNamespacesImpersonateAccessTokenFile, &transport.ImpersonationConfig{
			UserName: "system:serviceaccount:default:all-namespaces-read-access",
		})
		t.Run("grpc", func(t *testing.T) {
			_, err := gc.ListResults(ctx, &resultsv1alpha2.ListResultsRequest{Parent: p})
			if err != nil {
				t.Fatalf("Error listing Results: %v", err)
			}
		})
		t.Run("rest", func(t *testing.T) {
			_, err := rc.ListResults(ctx, &resultsv1alpha2.ListResultsRequest{Parent: p})
			if err != nil {
				t.Fatalf("Error listing Results: %v", err)
			}
		})
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
