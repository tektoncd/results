//go:build e2e && (blobs || gcs_blob)

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

// Shared helpers for blob log e2e tests (S3 and GCS variants).
package e2e

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	osexec "os/exec"
	"strings"
	"testing"
	"time"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/s3blob"
)

// newBlobHTTPClient creates an http.Client that trusts the e2e TLS cert.
func newBlobHTTPClient(t *testing.T) *http.Client {
	t.Helper()

	caCert, err := os.ReadFile(envCfg.CertFile)
	if err != nil {
		t.Fatalf("failed to read TLS cert %s: %v", envCfg.CertFile, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caCert) {
		t.Fatalf("failed to parse TLS cert from %s", envCfg.CertFile)
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    pool,
				ServerName: envCfg.ServerName,
				MinVersion: tls.VersionTLS12,
			},
		},
		Timeout: 120 * time.Second,
	}
}

// blobLogHTTPGet retrieves logs via the v1alpha3 plugin HTTP route.
// It reads the SA token for auth and makes a direct HTTPS call to the API
// server, returning the response body as a string.
func blobLogHTTPGet(t *testing.T, parent, resultID, recordID string) (int, string) {
	t.Helper()

	tokenPath := envCfg.TokenFile(allNamespacesReadAccessToken)
	token, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("failed to read token file %s: %v", tokenPath, err)
	}

	reqURL := fmt.Sprintf("%s/apis/results.tekton.dev/v1alpha3/parents/%s/results/%s/logs/%s",
		envCfg.ServerAddress, parent, resultID, recordID)

	httpClient := newBlobHTTPClient(t)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		t.Fatalf("failed to create HTTP request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	return resp.StatusCode, string(body)
}

// parseResultAndRecordIDs extracts the result UID and record UID from the
// full resource names returned by annotations.
// Result name format: <parent>/results/<uid>
// Record name format: <parent>/results/<uid>/records/<uid>
func parseResultAndRecordIDs(resultName, recordName string) (resultUID, recordUID string) {
	parts := strings.Split(resultName, "/")
	if len(parts) >= 3 {
		resultUID = parts[len(parts)-1]
	}
	parts = strings.Split(recordName, "/")
	if len(parts) >= 5 {
		recordUID = parts[len(parts)-1]
	}
	return resultUID, recordUID
}

// openSeededMinIOBucket port-forwards the in-cluster MinIO service to
// localhost:19001 and opens the tekton-logs bucket for direct seeding.
// Cleanup of the port-forward and bucket is registered via t.Cleanup so
// they outlive this helper (defer inside a helper would tear them down
// as soon as the helper returns).
func openSeededMinIOBucket(ctx context.Context, t *testing.T) *blob.Bucket {
	t.Helper()

	pfCmd := osexec.Command("kubectl", "port-forward", "svc/minio",
		"-n", "tekton-pipelines", "19001:9000")
	if err := pfCmd.Start(); err != nil {
		t.Fatalf("failed to start MinIO port-forward: %v", err)
	}
	t.Cleanup(func() { _ = pfCmd.Process.Kill() })

	t.Setenv("AWS_ACCESS_KEY_ID", "tekton-results")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "tekton-results-password")

	const (
		dialAddr = "localhost:19001"
		attempts = 30
	)
	var lastErr error
	for i := 1; i <= attempts; i++ {
		conn, err := net.DialTimeout("tcp", dialAddr, 2*time.Second)
		if err == nil {
			_ = conn.Close()
			lastErr = nil
			break
		}
		lastErr = err
		time.Sleep(1 * time.Second)
	}
	if lastErr != nil {
		t.Fatalf("MinIO not reachable via port-forward after %ds: %v", attempts, lastErr)
	}

	bucket, err := blob.OpenBucket(ctx,
		"s3://tekton-logs?region=us-east-1&endpoint=http://localhost:19001&s3ForcePathStyle=true&disable_https=true")
	if err != nil {
		t.Fatalf("failed to open S3 bucket: %v", err)
	}
	t.Cleanup(func() { _ = bucket.Close() })

	return bucket
}
