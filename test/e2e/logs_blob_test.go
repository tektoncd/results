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
// These tests exercise the log retrieval path through the v1alpha3 log
// plugin with blob storage backends (S3 via in-cluster MinIO, GCS via
// emulator). Test data is seeded directly into the blob store (bypassing
// Vector) because Vector's kubernetes_logs source is unreliable in kind
// CI clusters. The plugin HTTP route reads the data back — the API server
// never stores logs itself.
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

	resultsv1alpha2 "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"gocloud.dev/blob"
	_ "gocloud.dev/blob/s3blob"
)

// TestBlobLog_HappyPath verifies that a two-step TaskRun's logs can be
// retrieved via the v1alpha3 plugin HTTP route with correct per-container
// log identity.
//
// Data is seeded directly into MinIO (bypassing Vector) because Vector's
// log-forwarding pipeline is unreliable in kind CI clusters. Two separate
// objects are written (one per step) to verify the plugin concatenates
// them with the correct container headers.
func TestBlobLog_HappyPath(t *testing.T) {
	ctx := context.Background()

	gc, _ := resultsClient(t, allNamespacesAdminAccessToken, nil)

	res, err := gc.CreateResult(ctx, &resultsv1alpha2.CreateResultRequest{
		Parent: defaultNamespace,
		Result: &resultsv1alpha2.Result{
			Name: defaultNamespace + "/results/blob-log-test",
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
			Name: res.GetName() + "/records/blob-log-record",
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
	t.Logf("Result: %s (%s), Record: %s (%s)", res.GetName(), resultUID, rec.GetName(), recordUID)

	// Port-forward MinIO to the host so we can seed data directly.
	pfCmd := osexec.Command("kubectl", "port-forward", "svc/minio",
		"-n", "tekton-pipelines", "19001:9000")
	if err := pfCmd.Start(); err != nil {
		t.Fatalf("failed to start MinIO port-forward: %v", err)
	}
	defer func() { _ = pfCmd.Process.Kill() }()
	time.Sleep(3 * time.Second)

	t.Setenv("AWS_ACCESS_KEY_ID", "tekton-results")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "tekton-results-password")

	bucket, err := blob.OpenBucket(ctx,
		"s3://tekton-logs?region=us-east-1&endpoint=http://localhost:19001&s3ForcePathStyle=true&disable_https=true")
	if err != nil {
		t.Fatalf("failed to open S3 bucket: %v", err)
	}
	defer bucket.Close()

	// Seed two objects (one per step) matching the path format the blob
	// plugin expects: logs/{parent}/{resultName}/{recordName}/{container}.log
	// The plugin uses filepath.Split to extract the filename and
	// strings.TrimRight(file, ".log") as the container header.
	prefix := fmt.Sprintf("logs/%s/%s/%s", defaultNamespace, resultUID, recordUID)

	if err := bucket.WriteAll(ctx, prefix+"/step-first.log",
		[]byte("BLOB_LOG_MARKER_FIRST\n"), nil); err != nil {
		t.Fatalf("failed to seed step-first log: %v", err)
	}
	if err := bucket.WriteAll(ctx, prefix+"/step-second.log",
		[]byte("BLOB_LOG_MARKER_SECOND\n"), nil); err != nil {
		t.Fatalf("failed to seed step-second log: %v", err)
	}
	t.Logf("seeded two objects under %s/", prefix)

	// Read via the plugin HTTP route — data is already in MinIO.
	statusCode, body := blobLogHTTPGet(t, defaultNamespace, resultUID, recordUID)
	if statusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d; body: %.500s", statusCode, body)
	}
	if !strings.Contains(body, "BLOB_LOG_MARKER_FIRST") {
		t.Errorf("log body missing BLOB_LOG_MARKER_FIRST")
	}
	if !strings.Contains(body, "BLOB_LOG_MARKER_SECOND") {
		t.Errorf("log body missing BLOB_LOG_MARKER_SECOND")
	}

	// Verify per-container log identity. The blob plugin writes a
	// "{container} :-" header before each container's logs.
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

// TestBlobLog_PipelineRun verifies that logs for two different records
// under the same result are correctly isolated — reading one record's
// logs must not return another record's data.
//
// Data is seeded directly into MinIO (bypassing Vector) because Vector's
// log-forwarding pipeline is unreliable in kind CI clusters. This test
// focuses on the API's per-record log isolation.
func TestBlobLog_PipelineRun(t *testing.T) {
	ctx := context.Background()

	gc, _ := resultsClient(t, allNamespacesAdminAccessToken, nil)

	// Create one Result (simulating a PipelineRun) with two Records
	// (simulating two child TaskRuns).
	res, err := gc.CreateResult(ctx, &resultsv1alpha2.CreateResultRequest{
		Parent: defaultNamespace,
		Result: &resultsv1alpha2.Result{
			Name: defaultNamespace + "/results/blob-pipeline-test",
		},
	})
	if err != nil {
		t.Fatalf("CreateResult failed: %v", err)
	}
	t.Cleanup(func() {
		_, _ = gc.DeleteResult(ctx, &resultsv1alpha2.DeleteResultRequest{Name: res.GetName()})
	})

	recAlpha, err := gc.CreateRecord(ctx, &resultsv1alpha2.CreateRecordRequest{
		Parent: res.GetName(),
		Record: &resultsv1alpha2.Record{
			Name: res.GetName() + "/records/task-alpha-record",
			Data: &resultsv1alpha2.Any{
				Type:  "tekton.dev/v1.TaskRun",
				Value: []byte(`{}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateRecord (alpha) failed: %v", err)
	}

	recBeta, err := gc.CreateRecord(ctx, &resultsv1alpha2.CreateRecordRequest{
		Parent: res.GetName(),
		Record: &resultsv1alpha2.Record{
			Name: res.GetName() + "/records/task-beta-record",
			Data: &resultsv1alpha2.Any{
				Type:  "tekton.dev/v1.TaskRun",
				Value: []byte(`{}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateRecord (beta) failed: %v", err)
	}

	resultUID, alphaRecordUID := parseResultAndRecordIDs(res.GetName(), recAlpha.GetName())
	_, betaRecordUID := parseResultAndRecordIDs(res.GetName(), recBeta.GetName())

	// Port-forward MinIO to the host so we can seed data directly.
	pfCmd := osexec.Command("kubectl", "port-forward", "svc/minio",
		"-n", "tekton-pipelines", "19001:9000")
	if err := pfCmd.Start(); err != nil {
		t.Fatalf("failed to start MinIO port-forward: %v", err)
	}
	defer func() { _ = pfCmd.Process.Kill() }()
	time.Sleep(3 * time.Second)

	t.Setenv("AWS_ACCESS_KEY_ID", "tekton-results")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "tekton-results-password")

	bucket, err := blob.OpenBucket(ctx,
		"s3://tekton-logs?region=us-east-1&endpoint=http://localhost:19001&s3ForcePathStyle=true&disable_https=true")
	if err != nil {
		t.Fatalf("failed to open S3 bucket: %v", err)
	}
	defer bucket.Close()

	// Seed each record's log at the path the blob plugin expects:
	// logs/{parent}/{resultName}/{recordName}/{container}.log
	alphaKey := fmt.Sprintf("logs/%s/%s/%s/step-alpha.log",
		defaultNamespace, resultUID, alphaRecordUID)
	if err := bucket.WriteAll(ctx, alphaKey, []byte("PIPELINE_MARKER_ALPHA\n"), nil); err != nil {
		t.Fatalf("failed to seed alpha log: %v", err)
	}

	betaKey := fmt.Sprintf("logs/%s/%s/%s/step-beta.log",
		defaultNamespace, resultUID, betaRecordUID)
	if err := bucket.WriteAll(ctx, betaKey, []byte("PIPELINE_MARKER_BETA\n"), nil); err != nil {
		t.Fatalf("failed to seed beta log: %v", err)
	}

	t.Logf("seeded alpha at %s, beta at %s", alphaKey, betaKey)

	// Verify per-record isolation: each record returns only its own data.
	t.Run("task-alpha", func(t *testing.T) {
		code, body := blobLogHTTPGet(t, defaultNamespace, resultUID, alphaRecordUID)
		if code != http.StatusOK {
			t.Fatalf("expected HTTP 200, got %d; body: %s", code, body)
		}
		if !strings.Contains(body, "PIPELINE_MARKER_ALPHA") {
			t.Errorf("task-alpha log missing PIPELINE_MARKER_ALPHA; got: %s", body)
		}
		if strings.Contains(body, "PIPELINE_MARKER_BETA") {
			t.Errorf("task-alpha log contains PIPELINE_MARKER_BETA — cross-record leak")
		}
		t.Logf("task-alpha log: %d bytes", len(body))
	})

	t.Run("task-beta", func(t *testing.T) {
		code, body := blobLogHTTPGet(t, defaultNamespace, resultUID, betaRecordUID)
		if code != http.StatusOK {
			t.Fatalf("expected HTTP 200, got %d; body: %s", code, body)
		}
		if !strings.Contains(body, "PIPELINE_MARKER_BETA") {
			t.Errorf("task-beta log missing PIPELINE_MARKER_BETA; got: %s", body)
		}
		if strings.Contains(body, "PIPELINE_MARKER_ALPHA") {
			t.Errorf("task-beta log contains PIPELINE_MARKER_ALPHA — cross-record leak")
		}
		t.Logf("task-beta log: %d bytes", len(body))
	})
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

// TestBlobLog_LargeLog verifies that a large log (~4.5MB, 50k lines)
// stored in blob storage is retrieved completely without truncation.
//
// Data is seeded directly into MinIO (bypassing Vector) because Vector's
// log-forwarding pipeline is unreliable for larger payloads in
// resource-constrained kind CI clusters. This test focuses on the
// API's ability to stream large responses.
func TestBlobLog_LargeLog(t *testing.T) {
	ctx := context.Background()

	const lineCount = 50000

	// Create Result + Record directly via gRPC — no TaskRun needed.
	gc, _ := resultsClient(t, allNamespacesAdminAccessToken, nil)

	res, err := gc.CreateResult(ctx, &resultsv1alpha2.CreateResultRequest{
		Parent: defaultNamespace,
		Result: &resultsv1alpha2.Result{
			Name: defaultNamespace + "/results/blob-large-log",
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
			Name: res.GetName() + "/records/blob-large-record",
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
	t.Logf("Result: %s (%s), Record: %s (%s)", res.GetName(), resultUID, rec.GetName(), recordUID)

	// Generate ~4.5MB of test data: 50k lines × ~90 bytes each.
	var buf strings.Builder
	for i := 1; i <= lineCount; i++ {
		fmt.Fprintf(&buf, "LARGE_LOG_LINE_%d_PADDING_DATA_TO_INCREASE_SIZE_xxxxxxxxxxxxxxxxxxxxxxxxxx\n", i)
	}
	testData := buf.String()
	t.Logf("generated %d bytes (%.1f MB) of test data", len(testData), float64(len(testData))/(1024*1024))

	// Port-forward MinIO to the host so we can write directly.
	pfCmd := osexec.Command("kubectl", "port-forward", "svc/minio",
		"-n", "tekton-pipelines", "19001:9000")
	if err := pfCmd.Start(); err != nil {
		t.Fatalf("failed to start MinIO port-forward: %v", err)
	}
	defer func() { _ = pfCmd.Process.Kill() }()
	time.Sleep(3 * time.Second)

	// Open the bucket using the same gocloud.dev/blob SDK that the
	// API server uses for reading.
	t.Setenv("AWS_ACCESS_KEY_ID", "tekton-results")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "tekton-results-password")

	bucket, err := blob.OpenBucket(ctx,
		"s3://tekton-logs?region=us-east-1&endpoint=http://localhost:19001&s3ForcePathStyle=true&disable_https=true")
	if err != nil {
		t.Fatalf("failed to open S3 bucket: %v", err)
	}
	defer bucket.Close()

	// Write the test data at the path the blob plugin expects:
	//   logs/{parent}/{resultName}/{recordName}/{container}.log
	objectKey := fmt.Sprintf("logs/%s/%s/%s/step-generate.log",
		defaultNamespace, resultUID, recordUID)
	if err := bucket.WriteAll(ctx, objectKey, []byte(testData), nil); err != nil {
		t.Fatalf("failed to seed test data into MinIO: %v", err)
	}
	t.Logf("seeded %d bytes at %s", len(testData), objectKey)

	// Snapshot API pod memory before the large retrieval.
	memBefore := apiPodMemoryBytes(t)
	t.Logf("API pod memory before large-log retrieval: %d bytes (%.1f MB)",
		memBefore, float64(memBefore)/(1024*1024))

	// Read via the plugin HTTP route — data is already in MinIO.
	statusCode, body := blobLogHTTPGet(t, defaultNamespace, resultUID, recordUID)
	if statusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d; body: %.500s", statusCode, body)
	}

	// Verify first and last lines are present — no truncation.
	if !strings.Contains(body, "LARGE_LOG_LINE_1_") {
		t.Error("large log missing first line")
	}
	lastLine := fmt.Sprintf("LARGE_LOG_LINE_%d", lineCount)
	if !strings.Contains(body, lastLine) {
		t.Error("large log missing last line (truncated)")
	}
	t.Logf("large log retrieved: %d bytes (%.1f MB)", len(body), float64(len(body))/(1024*1024))

	// Log API pod memory delta for observability. A hard assertion is
	// omitted because kubelet stats are sampled and Go GC timing makes
	// precise memory checks unreliable in CI.
	memAfter := apiPodMemoryBytes(t)
	delta := memAfter - memBefore
	t.Logf("API pod memory after: %d bytes (%.1f MB), delta: %d bytes (%.1f MB)",
		memAfter, float64(memAfter)/(1024*1024),
		delta, float64(delta)/(1024*1024))
}
