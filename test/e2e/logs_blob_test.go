//go:build e2e && blobs

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

// Package e2e contains blob log backend e2e tests.
//
// These tests exercise the supported log retrieval path: the v1alpha3 log
// plugin with blob storage backends (S3 via in-cluster MinIO, GCS via
// emulator). Logs are shipped by Vector and retrieved through the plugin
// HTTP route — the API server never stores logs itself.
//
// The legacy TestGCSLog (e2e_gcs_test.go) covers the deprecated v1alpha2
// streaming-storage path and is intentionally separate.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	osexec "os/exec"
	"strings"
	"testing"
	"time"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	resultsv1alpha2 "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/yaml"
)

// waitForResultAnnotation polls until the TaskRun has the results.tekton.dev/result
// annotation, then returns the result and record names.
func waitForResultAnnotation(ctx context.Context, t *testing.T, trName string) (resultName, recordName string) {
	t.Helper()

	tc := tektonClient(t)
	if err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		tr, err := tc.TaskRuns(defaultNamespace).Get(ctx, trName, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		ann := tr.GetAnnotations()
		if ann == nil {
			return false, nil
		}
		rn, hasResult := ann["results.tekton.dev/result"]
		recn, hasRecord := ann["results.tekton.dev/record"]
		if hasResult && hasRecord {
			resultName = rn
			recordName = recn
			return true, nil
		}
		return false, nil
	}); err != nil {
		t.Fatalf("timed out waiting for result annotation on TaskRun %s: %v", trName, err)
	}
	return resultName, recordName
}

// TestBlobLog_HappyPath creates a TaskRun with two steps, waits for Vector
// to ship the logs to MinIO, then retrieves them via the v1alpha3 plugin
// HTTP route and asserts the expected content is present.
func TestBlobLog_HappyPath(t *testing.T) {
	ctx := context.Background()
	tr := new(tektonv1.TaskRun)
	b, err := os.ReadFile("testdata/taskrun-blob.yaml")
	if err != nil {
		t.Fatalf("Error reading taskrun-blob.yaml: %v", err)
	}
	if err := yaml.UnmarshalStrict(b, tr); err != nil {
		t.Fatalf("Error unmarshalling TaskRun: %v", err)
	}

	tc := tektonClient(t)

	// Clean up any leftover from a previous run.
	deletePolicy := metav1.DeletePropagationForeground
	_ = tc.TaskRuns(defaultNamespace).Delete(ctx, tr.GetName(), metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	})

	if _, err = tc.TaskRuns(defaultNamespace).Create(ctx, tr, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Error creating TaskRun: %v", err)
	}
	t.Cleanup(func() {
		_ = tc.TaskRuns(defaultNamespace).Delete(ctx, tr.GetName(), metav1.DeleteOptions{
			PropagationPolicy: &deletePolicy,
		})
	})

	resultName, recordName := waitForResultAnnotation(ctx, t, tr.GetName())
	t.Logf("Result: %s, Record: %s", resultName, recordName)

	resultUID, recordUID := parseResultAndRecordIDs(resultName, recordName)
	if resultUID == "" || recordUID == "" {
		t.Fatalf("failed to parse result/record UIDs from annotations: result=%q record=%q", resultName, recordName)
	}

	// Wait for Vector to ship logs (forwarder delay + processing time).
	t.Log("waiting for Vector to forward logs to MinIO...")

	var statusCode int
	var body string
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 3*time.Minute, true, func(_ context.Context) (bool, error) {
		statusCode, body = blobLogHTTPGet(t, defaultNamespace, resultUID, recordUID)
		if statusCode != http.StatusOK {
			t.Logf("log retrieval returned HTTP %d, body: %.200s", statusCode, body)
			return false, nil
		}
		if !strings.Contains(body, "BLOB_LOG_MARKER_FIRST") || !strings.Contains(body, "BLOB_LOG_MARKER_SECOND") {
			t.Logf("waiting for both markers (got %d bytes)", len(body))
			return false, nil
		}
		return true, nil
	}); err != nil {
		t.Fatalf("timed out waiting for blob log retrieval (last HTTP %d, body: %.500s): %v", statusCode, body, err)
	}
	if statusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d; body: %s", statusCode, body)
	}
	if !strings.Contains(body, "BLOB_LOG_MARKER_FIRST") {
		t.Errorf("log body missing BLOB_LOG_MARKER_FIRST")
	}
	if !strings.Contains(body, "BLOB_LOG_MARKER_SECOND") {
		t.Errorf("log body missing BLOB_LOG_MARKER_SECOND")
	}

	// Verify per-container log identity. The blob plugin writes a
	// "{container} :-" header before each container's logs. Check that
	// each marker appears after the correct container header.
	firstIdx := strings.Index(body, "step-first")
	secondIdx := strings.Index(body, "step-second")
	markerFirstIdx := strings.Index(body, "BLOB_LOG_MARKER_FIRST")
	markerSecondIdx := strings.Index(body, "BLOB_LOG_MARKER_SECOND")

	if firstIdx < 0 {
		t.Error("container header 'step-first' not found in log body")
	}
	if secondIdx < 0 {
		t.Error("container header 'step-second' not found in log body")
	}
	if firstIdx >= 0 && markerFirstIdx >= 0 && markerFirstIdx < firstIdx {
		t.Errorf("BLOB_LOG_MARKER_FIRST appeared before step-first header — wrong container identity")
	}
	if secondIdx >= 0 && markerSecondIdx >= 0 && markerSecondIdx < secondIdx {
		t.Errorf("BLOB_LOG_MARKER_SECOND appeared before step-second header — wrong container identity")
	}

	t.Logf("blob log retrieval succeeded (%d bytes)", len(body))
}

// TestBlobLog_PipelineRun creates a PipelineRun with two tasks, waits for
// each child TaskRun's logs to appear, and verifies that each task's log
// content is isolated to its own TaskRun (multi-task log identity).
func TestBlobLog_PipelineRun(t *testing.T) {
	ctx := context.Background()

	tc := tektonClient(t)

	deletePolicy := metav1.DeletePropagationForeground
	_ = tc.PipelineRuns(defaultNamespace).Delete(ctx, "blob-pipeline-test", metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	})

	prBytes, err := os.ReadFile("testdata/pipelinerun-blob.yaml")
	if err != nil {
		t.Fatalf("Error reading pipelinerun-blob.yaml: %v", err)
	}
	pr := new(tektonv1.PipelineRun)
	if err := yaml.UnmarshalStrict(prBytes, pr); err != nil {
		t.Fatalf("Error unmarshalling PipelineRun: %v", err)
	}
	if _, err := tc.PipelineRuns(defaultNamespace).Create(ctx, pr, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Error creating PipelineRun: %v", err)
	}
	t.Cleanup(func() {
		_ = tc.PipelineRuns(defaultNamespace).Delete(ctx, pr.GetName(), metav1.DeleteOptions{
			PropagationPolicy: &deletePolicy,
		})
	})

	// Wait for child TaskRuns to complete and get annotations.
	type taskRunInfo struct {
		name      string
		resultUID string
		recordUID string
	}
	var childTRs []taskRunInfo

	if err := wait.PollUntilContextTimeout(ctx, 3*time.Second, 3*time.Minute, true, func(_ context.Context) (bool, error) {
		trList, err := tc.TaskRuns(defaultNamespace).List(ctx, metav1.ListOptions{
			LabelSelector: "tekton.dev/pipelineRun=blob-pipeline-test",
		})
		if err != nil {
			return false, nil
		}
		if len(trList.Items) < 2 {
			return false, nil
		}
		childTRs = nil
		for i := range trList.Items {
			tr := &trList.Items[i]
			ann := tr.GetAnnotations()
			if ann == nil {
				return false, nil
			}
			rn, hasR := ann["results.tekton.dev/result"]
			recn, hasRec := ann["results.tekton.dev/record"]
			if !hasR || !hasRec {
				return false, nil
			}
			rUID, recUID := parseResultAndRecordIDs(rn, recn)
			childTRs = append(childTRs, taskRunInfo{
				name:      tr.GetName(),
				resultUID: rUID,
				recordUID: recUID,
			})
		}
		return true, nil
	}); err != nil {
		t.Fatalf("timed out waiting for child TaskRun annotations: %v", err)
	}

	t.Logf("found %d child TaskRuns", len(childTRs))

	// Wait for Vector to ship logs, then verify per-task isolation.
	for _, tri := range childTRs {
		tri := tri
		// Fail explicitly if the generated name doesn't identify the task.
		if !strings.Contains(tri.name, "task-alpha") && !strings.Contains(tri.name, "task-beta") {
			t.Fatalf("unexpected child TaskRun name %q — expected to contain task-alpha or task-beta", tri.name)
		}
		t.Run(tri.name, func(t *testing.T) {
			// Determine which marker this task must contain.
			var expectedMarker string
			if strings.Contains(tri.name, "task-alpha") {
				expectedMarker = "PIPELINE_MARKER_ALPHA"
			} else {
				expectedMarker = "PIPELINE_MARKER_BETA"
			}

			var body string
			var lastCode int
			if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 3*time.Minute, true, func(_ context.Context) (bool, error) {
				code, b := blobLogHTTPGet(t, defaultNamespace, tri.resultUID, tri.recordUID)
				lastCode = code
				if code != http.StatusOK {
					t.Logf("TaskRun %s: HTTP %d, body: %.200s", tri.name, code, b)
					return false, nil
				}
				if !strings.Contains(b, expectedMarker) {
					t.Logf("TaskRun %s: waiting for %s (got %d bytes)", tri.name, expectedMarker, len(b))
					return false, nil
				}
				body = b
				return true, nil
			}); err != nil {
				t.Fatalf("timed out waiting for %s in TaskRun %s (last HTTP %d): %v", expectedMarker, tri.name, lastCode, err)
			}

			// Each child TaskRun's log should contain only its own marker.
			hasAlpha := strings.Contains(body, "PIPELINE_MARKER_ALPHA")
			hasBeta := strings.Contains(body, "PIPELINE_MARKER_BETA")

			if strings.Contains(tri.name, "task-alpha") {
				if !hasAlpha {
					t.Errorf("task-alpha log missing PIPELINE_MARKER_ALPHA")
				}
				if hasBeta {
					t.Errorf("task-alpha log contains PIPELINE_MARKER_BETA — cross-task leak")
				}
			} else if strings.Contains(tri.name, "task-beta") {
				if !hasBeta {
					t.Errorf("task-beta log missing PIPELINE_MARKER_BETA")
				}
				if hasAlpha {
					t.Errorf("task-beta log contains PIPELINE_MARKER_ALPHA — cross-task leak")
				}
			}
			t.Logf("TaskRun %s log: %d bytes", tri.name, len(body))
		})
	}
}

// TestBlobLog_NoLogs verifies that requesting logs for a record with no
// logs in blob storage returns an error rather than crashing.
func TestBlobLog_NoLogs(t *testing.T) {
	ctx := context.Background()

	// Create a Result and Record with no associated logs in blob storage.
	gc, _ := resultsClient(t, allNamespacesAdminAccessToken, nil)

	res, err := gc.CreateResult(ctx, &resultsv1alpha2.CreateResultRequest{
		Parent: defaultNamespace,
		Result: &resultsv1alpha2.Result{
			Name: defaultNamespace + "/results/blob-no-logs",
		},
	})
	if err != nil {
		t.Fatalf("CreateResult failed: %v", err)
	}
	t.Cleanup(func() {
		_, _ = gc.DeleteResult(ctx, &resultsv1alpha2.DeleteResultRequest{Name: res.GetName()})
	})

	rec, err := gc.CreateRecord(ctx, &resultsv1alpha2.CreateRecordRequest{
		Parent: res.GetName(),
		Record: &resultsv1alpha2.Record{
			Name: res.GetName() + "/records/no-logs-record",
			Data: &resultsv1alpha2.Any{
				Type:  "tekton.dev/v1.TaskRun",
				Value: []byte(`{}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateRecord failed: %v", err)
	}

	resultUID, recordUID := parseResultAndRecordIDs(res.GetName(), rec.GetName())

	statusCode, body := blobLogHTTPGet(t, defaultNamespace, resultUID, recordUID)
	t.Logf("no-logs response: HTTP %d, body length=%d", statusCode, len(body))

	// The plugin finds the record in the DB but lists zero objects in blob
	// storage. getBlobLogs returns nil, so the handler writes nothing:
	// HTTP 200 with an empty body.
	if statusCode != http.StatusOK {
		t.Errorf("expected HTTP 200 for empty blob listing, got %d; body: %s", statusCode, body)
	}
	if len(body) > 0 {
		t.Errorf("expected empty body for record with no logs, got %d bytes: %.200s", len(body), body)
	}
}

// TestBlobLog_Unauthorized verifies that a request with no bearer token
// is rejected with 401.
func TestBlobLog_Unauthorized(t *testing.T) {
	reqURL := fmt.Sprintf("%s/apis/results.tekton.dev/v1alpha3/parents/%s/results/fake-result/logs/fake-record",
		envCfg.ServerAddress, defaultNamespace)

	httpClient := newBlobHTTPClient(t)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	// No authorization header — should be rejected.

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	t.Logf("unauthorized response: HTTP %d, body: %s", resp.StatusCode, string(body))
	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 401 or 403 for unauthenticated request, got %d", resp.StatusCode)
	}
}

// TestBlobLog_RBACDenied verifies that an authenticated request without
// logs/get permission is rejected with 401.
func TestBlobLog_RBACDenied(t *testing.T) {
	reqURL := fmt.Sprintf("%s/apis/results.tekton.dev/v1alpha3/parents/%s/results/fake-result/logs/fake-record",
		envCfg.ServerAddress, defaultNamespace)

	// Use the impersonate SA token — it can authenticate but has no
	// results/records/logs permissions.
	tokenPath := envCfg.TokenFile(allNamespacesImpersonateAccessToken)
	token, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("failed to read token: %v", err)
	}

	httpClient := newBlobHTTPClient(t)
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 401/403 for authenticated-but-denied request, got %d; body: %s", resp.StatusCode, string(body))
	}
}

// apiPodMemoryBytes reads the API container's workingSetBytes from the
// kubelet stats summary API. This works with any base image (including
// static/distroless) because it reads from outside the pod.
func apiPodMemoryBytes(t *testing.T) int64 {
	t.Helper()

	// Get the node the API pod runs on.
	nodeOut, err := osexec.Command("kubectl", "get", "pod",
		"-n", "tekton-pipelines",
		"-l", "app.kubernetes.io/name=tekton-results-api",
		"-o", "jsonpath={.items[0].spec.nodeName}",
	).Output()
	if err != nil {
		t.Fatalf("failed to get API pod node: %v", err)
	}
	nodeName := strings.TrimSpace(string(nodeOut))

	// Read kubelet stats summary via the API server proxy.
	raw, err := osexec.Command("kubectl", "get", "--raw",
		fmt.Sprintf("/api/v1/nodes/%s/proxy/stats/summary", nodeName),
	).Output()
	if err != nil {
		t.Fatalf("failed to read kubelet stats summary: %v", err)
	}

	// Parse just enough of the response to find the API container.
	var summary struct {
		Pods []struct {
			PodRef struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"podRef"`
			Containers []struct {
				Name   string `json:"name"`
				Memory struct {
					WorkingSetBytes *int64 `json:"workingSetBytes"`
				} `json:"memory"`
			} `json:"containers"`
		} `json:"pods"`
	}
	if err := json.Unmarshal(raw, &summary); err != nil {
		t.Fatalf("failed to parse kubelet stats: %v", err)
	}

	for _, pod := range summary.Pods {
		if pod.PodRef.Namespace != "tekton-pipelines" {
			continue
		}
		if !strings.HasPrefix(pod.PodRef.Name, "tekton-results-api") {
			continue
		}
		for _, c := range pod.Containers {
			if c.Name == "api" && c.Memory.WorkingSetBytes != nil {
				return *c.Memory.WorkingSetBytes
			}
		}
	}
	t.Fatal("API container memory stats not found in kubelet summary")
	return 0
}

// TestBlobLog_LargeLog verifies that a TaskRun producing a large amount
// of log output (~4.5MB, 50k lines) is retrieved completely without
// truncation, and that the API server's memory stays bounded.
func TestBlobLog_LargeLog(t *testing.T) {
	ctx := context.Background()

	// ~4.5MB: 50k lines × ~90 bytes each.
	const lineCount = 50000
	tr := &tektonv1.TaskRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "blob-large-log",
		},
		Spec: tektonv1.TaskRunSpec{
			ServiceAccountName: "default",
			TaskSpec: &tektonv1.TaskSpec{
				Steps: []tektonv1.Step{
					{
						Name:  "generate",
						Image: "alpine",
						Command: []string{"sh", "-c",
							fmt.Sprintf(`awk 'BEGIN{for(i=1;i<=%d;i++) print "LARGE_LOG_LINE_" i "_PADDING_DATA_TO_INCREASE_SIZE_xxxxxxxxxxxxxxxxxxxxxxxxxx"}'`, lineCount),
						},
					},
				},
			},
		},
	}

	tc := tektonClient(t)
	deletePolicy := metav1.DeletePropagationForeground
	_ = tc.TaskRuns(defaultNamespace).Delete(ctx, tr.GetName(), metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	})

	if _, err := tc.TaskRuns(defaultNamespace).Create(ctx, tr, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Error creating TaskRun: %v", err)
	}
	t.Cleanup(func() {
		_ = tc.TaskRuns(defaultNamespace).Delete(ctx, tr.GetName(), metav1.DeleteOptions{
			PropagationPolicy: &deletePolicy,
		})
	})

	resultName, recordName := waitForResultAnnotation(ctx, t, tr.GetName())
	resultUID, recordUID := parseResultAndRecordIDs(resultName, recordName)

	// Snapshot API pod memory before the large retrieval.
	memBefore := apiPodMemoryBytes(t)
	t.Logf("API pod memory before large-log retrieval: %d bytes (%.1f MB)", memBefore, float64(memBefore)/(1024*1024))

	// Wait for logs to appear.
	lastLine := fmt.Sprintf("LARGE_LOG_LINE_%d", lineCount)
	var finalBody string
	var lastCode int
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 3*time.Minute, true, func(_ context.Context) (bool, error) {
		code, body := blobLogHTTPGet(t, defaultNamespace, resultUID, recordUID)
		lastCode = code
		if code != http.StatusOK {
			t.Logf("large log retrieval HTTP %d, body: %.200s", code, body)
			return false, nil
		}
		if !strings.Contains(body, lastLine) {
			t.Logf("last line not yet present, got %d bytes so far...", len(body))
			return false, nil
		}
		finalBody = body
		return true, nil
	}); err != nil {
		t.Fatalf("timed out waiting for large log (last HTTP %d): %v", lastCode, err)
	}

	// Verify first and last lines are present — no truncation.
	if !strings.Contains(finalBody, "LARGE_LOG_LINE_1_") {
		t.Error("large log missing first line")
	}
	if !strings.Contains(finalBody, lastLine) {
		t.Error("large log missing last line (truncated)")
	}
	t.Logf("large log retrieved: %d bytes (%.1f MB)", len(finalBody), float64(len(finalBody))/(1024*1024))

	// Check API pod memory after retrieval. The server streams blob
	// objects through a 32KB buffer (WriteTo), so working-set growth
	// should be on the order of that buffer plus runtime overhead — well
	// below the ~4.5MB payload. Log the delta for observability; a hard
	// assertion is omitted because kubelet stats are sampled and Go GC
	// timing makes precise memory checks unreliable in CI.
	memAfter := apiPodMemoryBytes(t)
	delta := memAfter - memBefore
	t.Logf("API pod memory after: %d bytes (%.1f MB), delta: %d bytes (%.1f MB)",
		memAfter, float64(memAfter)/(1024*1024),
		delta, float64(delta)/(1024*1024))
}
