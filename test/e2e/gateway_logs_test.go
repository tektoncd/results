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

//go:build e2e && gateway_logs

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tektoncd/results/pkg/apis/v1alpha3"
)

func TestGatewayLegacyLogs(t *testing.T) {
	expectedFile := os.Getenv("GATEWAY_LOG_EXPECTED_FILE")
	if expectedFile == "" {
		t.Fatal("set GATEWAY_LOG_EXPECTED_FILE to the known deployed file-backend fixture")
	}
	expected, err := os.ReadFile(expectedFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(expected) < 64*1024 {
		t.Fatal("fixture must span multiple legacy streaming chunks")
	}
	logFile := os.Getenv("GATEWAY_LEGACY_LOG_FILE")
	if logFile == "" {
		logFile = "/tmp/gateway-logs/log"
	}
	data, err := json.Marshal(&v1alpha3.Log{Spec: v1alpha3.LogSpec{Type: v1alpha3.FileLogType, Resource: v1alpha3.Resource{Namespace: defaultNamespace, Name: "gateway-log"}}, Status: v1alpha3.LogStatus{Path: logFile, Size: int64(len(expected)), IsStored: true}})
	if err != nil {
		t.Fatal(err)
	}
	_, res, record := deployedGatewayFixture(t, v1alpha3.LogRecordType, data)
	path := strings.Replace(record.Name, "/records/", "/logs/", 1)
	base := strings.TrimRight(envCfg.ServerAddress, "/") + "/apis/results.tekton.dev/v1alpha2/parents/"
	hc := deployedGatewayHTTP(t, allNamespacesAdminAccessToken)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("legacy stream: %d %s", resp.StatusCode, body)
	}
	first := make([]byte, 1024)
	if _, err := io.ReadFull(resp.Body, first); err != nil {
		resp.Body.Close()
		t.Fatal(err)
	}
	rest, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	got := bytes.ReplaceAll(append(first, rest...), []byte("\n"), nil)
	if resp.StatusCode != 200 || !bytes.Equal(got, bytes.ReplaceAll(expected, []byte("\n"), nil)) {
		t.Fatalf("legacy stream status=%d bytes=%d", resp.StatusCode, len(got))
	}
	streamCtx, stopStream := context.WithCancel(ctx)
	req, err = http.NewRequestWithContext(streamCtx, "GET", base+path, nil)
	if err != nil {
		stopStream()
		t.Fatal(err)
	}
	resp, err = hc.Do(req)
	if err != nil {
		stopStream()
		t.Fatal(err)
	}
	if _, err := io.ReadFull(resp.Body, first); err != nil {
		stopStream()
		resp.Body.Close()
		t.Fatal(err)
	}
	stopStream()
	resp.Body.Close()
	for _, op := range []struct {
		method, path string
		want         int
	}{{"GET", res.Name + "/logs", 200}, {"DELETE", path, 200}, {"GET", path, 404}} {
		req, err := http.NewRequestWithContext(ctx, op.method, base+op.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := hc.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != op.want {
			t.Fatalf("%s %s: %d %s", op.method, op.path, response.StatusCode, body)
		}
	}
}

func TestGatewayPluginLogs(t *testing.T) {
	pushURL := os.Getenv("GATEWAY_LOKI_PUSH_URL")
	if pushURL == "" {
		t.Fatal("set GATEWAY_LOKI_PUSH_URL to a prepared local Loki push endpoint")
	}
	now := time.Now().UTC()
	data, err := json.Marshal(map[string]any{"status": map[string]string{"startTime": now.Add(-time.Minute).Format(time.RFC3339), "completionTime": now.Add(time.Minute).Format(time.RFC3339)}})
	if err != nil {
		t.Fatal(err)
	}
	_, _, record := deployedGatewayFixture(t, "tekton.dev/v1.PipelineRun", data)
	name := record.Name[strings.LastIndex(record.Name, "/")+1:]
	expected := "known plugin content " + name
	line, err := json.Marshal(map[string]any{"kubernetes": map[string]any{"labels": map[string]string{"tekton_dev_pipelineRunUID": name}}, "message": expected})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{"streams": []any{map[string]any{"stream": map[string]string{"namespace": defaultNamespace, "log_type": "application"}, "values": [][]string{{strconv.FormatInt(now.UnixNano(), 10), string(line)}}}}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", pushURL, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 204 {
		t.Fatalf("Loki push: %d %s", response.StatusCode, body)
	}
	hc := deployedGatewayHTTP(t, allNamespacesAdminAccessToken)
	anonymous := deployedGatewayHTTP(t, "")
	for _, version := range []string{"v1alpha2", "v1alpha3"} {
		endpoint := strings.TrimRight(envCfg.ServerAddress, "/") + "/apis/results.tekton.dev/" + version + "/parents/" + strings.Replace(record.Name, "/records/", "/logs/", 1)
		req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
		if err != nil {
			t.Fatal(err)
		}
		denied, err := anonymous.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, denied.Body)
		denied.Body.Close()
		if denied.StatusCode != http.StatusUnauthorized {
			t.Fatalf("anonymous plugin %s: %d", version, denied.StatusCode)
		}
		for {
			req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := hc.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != 200 {
				t.Fatalf("plugin %s: %d %s", version, response.StatusCode, body)
			}
			if strings.Contains(string(body), expected) {
				break
			}
			select {
			case <-ctx.Done():
				t.Fatal("known backend content not retrieved")
			case <-time.After(200 * time.Millisecond):
			}
		}
	}
}
