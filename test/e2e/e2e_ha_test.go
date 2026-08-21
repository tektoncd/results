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

//go:build e2e && e2e_ha
// +build e2e,e2e_ha

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	resultsv1alpha2 "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

const (
	haNamespace        = "tekton-pipelines"
	haAPIBasename      = "tekton-results-api"
	haWatcherBasename  = "tekton-results-watcher"
	haPostgresBasename = "tekton-results-postgres"
	haExpectedReplicas = 3
)

// TestHorizontalScaling tests the HA configuration correctness.
// It verifies three critical properties:
// 1. No duplicated gRPC requests - each TaskRun reconciled by exactly one watcher
// 2. No lost requests - every TaskRun gets a Result and Record persisted and retrievable
// 3. API pod distribution - gRPC requests spread across multiple API pods via round_robin
func TestHorizontalScaling(t *testing.T) {
	ctx := context.Background()
	tc := tektonClient(t)
	gc, _ := resultsClient(t, allNamespacesReadAccessTokenFile, nil)

	t.Run("VerifyPods", func(t *testing.T) {
		// Verify 3 API pods are ready
		t.Run("API replicas", func(t *testing.T) {
			verifyPodsReady(ctx, t, haNamespace, haAPIBasename, haExpectedReplicas)
		})

		// Verify 3 watcher pods are ready
		t.Run("Watcher replicas", func(t *testing.T) {
			verifyPodsReady(ctx, t, haNamespace, haWatcherBasename, haExpectedReplicas)
		})

		// Verify postgres pod is ready
		t.Run("Postgres replica", func(t *testing.T) {
			verifyPodsReady(ctx, t, haNamespace, haPostgresBasename, 1)
		})
	})

	// Setup: create 9 TaskRuns and wait for annotations
	const numTaskRuns = 9
	taskRunNames := make([]string, numTaskRuns)
	for i := 0; i < numTaskRuns; i++ {
		taskRunNames[i] = fmt.Sprintf("ha-guarantee-taskrun-%d", i)
	}

	// Clean up any existing TaskRuns and wait for deletion to complete
	for _, name := range taskRunNames {
		_ = tc.TaskRuns(defaultNamespace).Delete(ctx, name, metav1.DeleteOptions{})
	}
	// Wait for all TaskRuns to be fully deleted
	for _, name := range taskRunNames {
		_ = wait.PollUntilContextTimeout(ctx, 1*time.Second, 30*time.Second, true, func(ctx context.Context) (done bool, err error) {
			_, err = tc.TaskRuns(defaultNamespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				// TaskRun not found means it's deleted
				return true, nil
			}
			return false, nil
		})
	}

	// Create TaskRuns
	t.Log("Creating 9 TaskRuns for HA testing")
	for _, name := range taskRunNames {
		tr := &tektonv1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: tektonv1.TaskRunSpec{
				ServiceAccountName: "default",
				TaskSpec: &tektonv1.TaskSpec{
					Steps: []tektonv1.Step{
						{
							Name:    "echo",
							Image:   "alpine",
							Command: []string{"echo", fmt.Sprintf("hello from %s", name)},
						},
					},
				},
			},
		}

		_, err := tc.TaskRuns(defaultNamespace).Create(ctx, tr, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Error creating TaskRun %s: %v", name, err)
		}
		t.Logf("Created TaskRun %s", name)
	}

	// Defer cleanup
	defer func() {
		for _, name := range taskRunNames {
			_ = tc.TaskRuns(defaultNamespace).Delete(ctx, name, metav1.DeleteOptions{})
		}
	}()

	// Wait for all TaskRuns to get Result/Record annotations
	type taskRunAnnotations struct {
		resultName string
		recordName string
	}
	annotations := make(map[string]taskRunAnnotations, numTaskRuns)

	t.Log("Waiting for all TaskRuns to receive Result and Record annotations")
	for _, name := range taskRunNames {
		var resName, recName string

		err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 2*time.Minute, true, func(ctx context.Context) (done bool, err error) {
			tr, err := tc.TaskRuns(defaultNamespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				t.Logf("Error getting TaskRun %s: %v", name, err)
				return false, nil
			}

			var resAnnotation, recAnnotation bool
			resName, resAnnotation = tr.GetAnnotations()["results.tekton.dev/result"]
			recName, recAnnotation = tr.GetAnnotations()["results.tekton.dev/record"]

			if resAnnotation && recAnnotation {
				return true, nil
			}
			return false, nil
		})

		if err != nil {
			t.Fatalf("TaskRun %s did not get Result/Record annotations within timeout", name)
		}

		annotations[name] = taskRunAnnotations{
			resultName: resName,
			recordName: recName,
		}
		t.Logf("TaskRun %s annotated with Result: %s, Record: %s", name, resName, recName)
	}

	t.Run("NoDuplicates", func(t *testing.T) {
		t.Run("API-side check", func(t *testing.T) {
			// For each TaskRun's Result, list its Records and count only TaskRun-type records.
			// A single Result may contain multiple record types (TaskRun data, log, event list),
			// so we filter by Data.Type to check for TaskRun duplicates specifically.
			for trName, annot := range annotations {
				resp, err := gc.ListRecords(ctx, &resultsv1alpha2.ListRecordsRequest{
					Parent: annot.resultName,
				})
				if err != nil {
					t.Errorf("TaskRun %s: error listing records for Result %s: %v", trName, annot.resultName, err)
					continue
				}

				taskRunRecordCount := 0
				for _, rec := range resp.Records {
					if rec.GetData() != nil && strings.Contains(rec.GetData().GetType(), "TaskRun") {
						taskRunRecordCount++
					}
				}

				if taskRunRecordCount != 1 {
					t.Errorf("TaskRun %s: expected exactly 1 TaskRun Record for Result %s, got %d (total records: %d)", trName, annot.resultName, taskRunRecordCount, len(resp.Records))
				} else {
					t.Logf("TaskRun %s: 1 TaskRun Record, %d total records (includes logs/events)", trName, len(resp.Records))
				}
			}
		})

		t.Run("Watcher-side check", func(t *testing.T) {
			// Read logs from all watcher pods and verify each TaskRun appears in exactly one pod's logs
			clientset := clientConfig(t)
			k8sClient := kubernetes.NewForConfigOrDie(clientset)

			pods, err := k8sClient.CoreV1().Pods(haNamespace).List(ctx, metav1.ListOptions{})
			if err != nil {
				t.Fatalf("Error listing pods: %v", err)
			}

			// Map TaskRun name -> list of watcher pods that reconciled it
			taskRunToWatchers := make(map[string][]string)

			// Read logs from each watcher pod
			for _, pod := range pods.Items {
				if !startsWithPrefix(pod.Name, haWatcherBasename) {
					continue
				}

				req := k8sClient.CoreV1().Pods(haNamespace).GetLogs(pod.Name, &corev1.PodLogOptions{})
				stream, err := req.Stream(ctx)
				if err != nil {
					t.Errorf("Error getting logs for watcher pod %s: %v", pod.Name, err)
					continue
				}

				logsBytes, err := io.ReadAll(stream)
				stream.Close()
				if err != nil {
					t.Errorf("Error reading logs for watcher pod %s: %v", pod.Name, err)
					continue
				}

				logs := string(logsBytes)

				// Check which TaskRuns appear in this pod's logs
				for _, trName := range taskRunNames {
					// Look for log entries with "knative.dev/key":"default/<taskrun-name>"
					keyPattern := fmt.Sprintf(`"knative.dev/key":"default/%s"`, trName)
					if strings.Contains(logs, keyPattern) {
						taskRunToWatchers[trName] = append(taskRunToWatchers[trName], pod.Name)
					}
				}
			}

			// Assert each TaskRun was reconciled by exactly 1 watcher
			for _, trName := range taskRunNames {
				watchers := taskRunToWatchers[trName]
				if len(watchers) == 0 {
					t.Errorf("TaskRun %s: not found in any watcher pod logs", trName)
				} else if len(watchers) > 1 {
					t.Errorf("TaskRun %s: found in multiple watcher pod logs: %v", trName, watchers)
				} else {
					t.Logf("TaskRun %s: reconciled exclusively by %s", trName, watchers[0])
				}
			}
		})
	})

	t.Run("NoLostRequests", func(t *testing.T) {
		t.Run("Count invariant", func(t *testing.T) {
			// List all records and verify all TaskRuns from this test run are present.
			// Build a set of expected record names from the current test's annotations.
			expectedRecords := make(map[string]bool)
			for _, annot := range annotations {
				expectedRecords[annot.recordName] = true
			}

			// Paginate through all records to avoid hitting gRPC message size limits
			count := 0
			pageToken := ""
			for {
				resp, err := gc.ListRecords(ctx, &resultsv1alpha2.ListRecordsRequest{
					Parent:    "default/results/-",
					PageSize:  100,
					PageToken: pageToken,
				})
				if err != nil {
					t.Fatalf("Error listing records (page token: %s): %v", pageToken, err)
				}

				for _, record := range resp.Records {
					// Only count records that belong to this test run
					if !expectedRecords[record.Name] {
						continue
					}

					// Verify it's a TaskRun-type record
					if record.GetData() == nil || !strings.Contains(record.GetData().GetType(), "TaskRun") {
						t.Errorf("Record %s from current test is not a TaskRun type: %s", record.Name, record.GetData().GetType())
						continue
					}

					count++
				}

				// Check if there are more pages
				if resp.NextPageToken == "" {
					break
				}
				pageToken = resp.NextPageToken
			}

			if count != numTaskRuns {
				t.Errorf("Expected %d TaskRun records from current test run, got %d", numTaskRuns, count)
			} else {
				t.Logf("Count check passed: %d TaskRun records found", count)
			}
		})

		t.Run("Individual retrieval", func(t *testing.T) {
			// Verify each annotated Record is individually retrievable via GetRecord
			for trName, annot := range annotations {
				_, err := gc.GetRecord(ctx, &resultsv1alpha2.GetRecordRequest{
					Name: annot.recordName,
				})
				if err != nil {
					t.Errorf("TaskRun %s: failed to retrieve Record %s: %v", trName, annot.recordName, err)
				}
			}
			t.Logf("All %d records individually retrievable", len(annotations))
		})

		t.Run("Data integrity", func(t *testing.T) {
			// Verify each Record's data contains the correct TaskRun name
			for trName, annot := range annotations {
				record, err := gc.GetRecord(ctx, &resultsv1alpha2.GetRecordRequest{
					Name: annot.recordName,
				})
				if err != nil {
					t.Errorf("TaskRun %s: failed to get Record: %v", trName, err)
					continue
				}

				var tr tektonv1.TaskRun
				if err := json.Unmarshal(record.Data.Value, &tr); err != nil {
					t.Errorf("TaskRun %s: failed to unmarshal Record data: %v", trName, err)
					continue
				}

				if tr.Name != trName {
					t.Errorf("TaskRun %s: Record data contains wrong TaskRun name: %s", trName, tr.Name)
				}
			}
			t.Logf("All records have correct TaskRun names")
		})
	})

	t.Run("APIDistribution", func(t *testing.T) {
		// Read logs from all API pods and count gRPC requests per pod
		clientset := clientConfig(t)
		k8sClient := kubernetes.NewForConfigOrDie(clientset)

		pods, err := k8sClient.CoreV1().Pods(haNamespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatalf("Error listing pods: %v", err)
		}

		requestCountPerPod := make(map[string]int)

		for _, pod := range pods.Items {
			if !startsWithPrefix(pod.Name, haAPIBasename) {
				continue
			}

			req := k8sClient.CoreV1().Pods(haNamespace).GetLogs(pod.Name, &corev1.PodLogOptions{})
			stream, err := req.Stream(ctx)
			if err != nil {
				t.Errorf("Error getting logs for API pod %s: %v", pod.Name, err)
				continue
			}

			logsBytes, err := io.ReadAll(stream)
			stream.Close()
			if err != nil {
				t.Errorf("Error reading logs for API pod %s: %v", pod.Name, err)
				continue
			}

			logs := string(logsBytes)
			lines := strings.Split(logs, "\n")

			count := 0
			for _, line := range lines {
				if strings.Contains(line, `"grpc.method"`) {
					count++
				}
			}

			requestCountPerPod[pod.Name] = count
			t.Logf("API pod %s: %d gRPC requests", pod.Name, count)
		}

		// Assert at least 2 of 3 API pods received requests
		podsWithRequests := 0
		for _, count := range requestCountPerPod {
			if count > 0 {
				podsWithRequests++
			}
		}

		if podsWithRequests < 2 {
			t.Errorf("Expected at least 2 API pods to receive requests, but only %d pods received requests", podsWithRequests)
			t.Logf("Request distribution: %v", requestCountPerPod)
		} else {
			t.Logf("Distribution check passed: %d of %d API pods received requests", podsWithRequests, len(requestCountPerPod))
		}
	})
}

// verifyPodsReady waits for the expected number of pods with the given basename to be ready.
func verifyPodsReady(ctx context.Context, t *testing.T, namespace, podBasename string, expectedCount int) {
	t.Helper()

	clientset := clientConfig(t)
	k8sClient := kubernetes.NewForConfigOrDie(clientset)

	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 2*time.Minute, true, func(ctx context.Context) (done bool, err error) {
		pods, err := k8sClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Logf("Error listing pods: %v", err)
			return false, nil
		}

		readyCount := 0
		for _, pod := range pods.Items {
			// Check if pod name starts with the expected basename
			if !startsWithPrefix(pod.Name, podBasename) {
				continue
			}

			// Check if pod is ready
			if isPodReady(&pod) {
				readyCount++
			}
		}

		if readyCount >= expectedCount {
			t.Logf("Found %d/%d ready pods with basename %s", readyCount, expectedCount, podBasename)
			return true, nil
		}

		t.Logf("Waiting for pods with basename %s: %d/%d ready", podBasename, readyCount, expectedCount)
		return false, nil
	})

	if err != nil {
		t.Fatalf("Pods with basename %s did not become ready: %v", podBasename, err)
	}
}

// startsWithPrefix checks if a string starts with a given prefix.
func startsWithPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// isPodReady checks if a pod is in Ready state.
func isPodReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
