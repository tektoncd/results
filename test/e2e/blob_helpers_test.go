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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
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
