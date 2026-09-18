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

//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	resultsv1alpha2 "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"github.com/tektoncd/results/test/e2e/client"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8suuid "k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
)

const (
	pluginLogPathFmt = "/apis/results.tekton.dev/v1alpha3/parents/%s/results/%s/logs/%s"

	// Must match LOGGING_PLUGIN_FORWARDER_DELAY_DURATION in
	// test/e2e/loki_vector/loki-vector-api-config.yaml (that value is minutes).
	lokiForwarderDelay = 1 * time.Minute

	// Must match LOGGING_PLUGIN_FORWARDER_DELAY_DURATION in
	// test/e2e/splunk/api-config.yaml (that value is minutes).
	splunkForwarderDelay = 5 * time.Minute
)

// uniquePluginRecordUID is a new Result/Record name each run so CreateResult
// does not hit results_by_name on re-runs against the same DB.
func uniquePluginRecordUID() string {
	return string(k8suuid.NewUUID())
}

// logMarkers must match testdata/pipelinerun-plugin-logs.yaml
var logMarkers = []string{
	"PLUGIN_LOG_E2E_MARKER_ALPHA",
	"PLUGIN_LOG_E2E_MARKER_BETA",
	"PLUGIN_LOG_E2E_MARKER_GAMMA",
}

type pluginLogResponse struct {
	StatusCode int
	Body       string
	Duration   time.Duration
}

func loadPluginLogPipelineRun(t *testing.T, name string) *tektonv1.PipelineRun {
	t.Helper()
	b, err := os.ReadFile("testdata/pipelinerun-plugin-logs.yaml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	pr := new(tektonv1.PipelineRun)
	if err := yaml.UnmarshalStrict(b, pr); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	pr.Name = name
	pr.Namespace = defaultNamespace
	return pr
}

func createAndWaitPluginLogPipelineRun(t *testing.T, ctx context.Context, name string) (resultName, recordName string, pr *tektonv1.PipelineRun) {
	t.Helper()
	pr = loadPluginLogPipelineRun(t, name)
	tc := tektonClient(t)

	_ = tc.PipelineRuns(defaultNamespace).Delete(ctx, pr.GetName(), metav1.DeleteOptions{})

	created, err := tc.PipelineRuns(defaultNamespace).Create(ctx, pr, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create PipelineRun: %v", err)
	}

	if err := wait.PollUntilContextTimeout(ctx, time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		got, err := tc.PipelineRuns(defaultNamespace).Get(ctx, created.Name, metav1.GetOptions{})
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		res, okRes := got.GetAnnotations()["results.tekton.dev/result"]
		rec, okRec := got.GetAnnotations()["results.tekton.dev/record"]
		if okRes && okRec {
			resultName, recordName = res, rec
			pr = got
			return true, nil
		}
		return false, nil
	}); err != nil {
		t.Fatalf("waiting for results annotations: %v", err)
	}

	return resultName, recordName, pr
}

func pluginLogURL(resultName, recordName string) (parent, resultID, recordID, u string) {
	parts := strings.Split(resultName, "/")
	recParts := strings.Split(recordName, "/")
	if len(parts) < 3 || len(recParts) < 1 {
		return "", "", "", ""
	}
	parent, resultID = parts[0], parts[2]
	recordID = recParts[len(recParts)-1]
	path := fmt.Sprintf(pluginLogPathFmt, parent, resultID, recordID)
	u = strings.TrimRight(serverAddress, "/") + path
	return parent, resultID, recordID, u
}

func getPluginLogs(t *testing.T, ctx context.Context, tokenFile, rawURL string) pluginLogResponse {
	return getPluginLogsWithTimeout(t, ctx, tokenFile, rawURL, 20*time.Second)
}

func getPluginLogsWithTimeout(t *testing.T, ctx context.Context, tokenFile, rawURL string, timeout time.Duration) pluginLogResponse {
	t.Helper()
	token, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Fatalf("read token: %v", err)
	}

	reqCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Host = serverName
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))

	httpClient := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // e2e kind cert
		},
	}

	start := time.Now()
	resp, err := httpClient.Do(req)
	dur := time.Since(start)
	if err != nil {
		t.Fatalf("plugin log GET: %v (duration %s)", err, dur)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return pluginLogResponse{StatusCode: resp.StatusCode, Body: string(body), Duration: dur}
}

func waitForPluginLogContent(t *testing.T, ctx context.Context, tokenFile, rawURL string, wantSubstrings []string, timeout time.Duration) pluginLogResponse {
	t.Helper()
	var last pluginLogResponse
	err := wait.PollUntilContextTimeout(ctx, 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		last = getPluginLogs(t, ctx, tokenFile, rawURL)
		if last.StatusCode == http.StatusNotFound && strings.Contains(last.Body, `"code":5`) {
			t.Fatalf("v1alpha3 log plugin route is not registered (API LOGS_TYPE is not Loki/Splunk/blob). last status=%d body=%q", last.StatusCode, last.Body)
		}
		if last.StatusCode != http.StatusOK {
			t.Logf("plugin GET status=%d body=%q", last.StatusCode, last.Body)
			return false, nil
		}
		for _, s := range wantSubstrings {
			if !strings.Contains(last.Body, s) {
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("logs never contained %v; last status=%d body=%q: %v", wantSubstrings, last.StatusCode, last.Body, err)
	}
	return last
}

func resultsAPIPodLogs(t *testing.T, ctx context.Context, since time.Time) string {
	t.Helper()
	mt := metav1.NewTime(since)
	logs, err := getResultsAPILogs(ctx, &corev1.PodLogOptions{SinceTime: &mt}, t)
	if err != nil {
		t.Fatalf("api logs: %v", err)
	}
	return logs
}

// storedPipelineRun is the object the plugin unmarshals in getLogRequestParams.
func storedPipelineRun(t *testing.T, ctx context.Context, gc client.GRPCClient, recordName string) *tektonv1.PipelineRun {
	t.Helper()
	rec, err := gc.GetRecord(ctx, &resultsv1alpha2.GetRecordRequest{Name: recordName})
	if err != nil {
		t.Fatalf("GetRecord: %v", err)
	}
	var stored tektonv1.PipelineRun
	if err := json.Unmarshal(rec.Data.Value, &stored); err != nil {
		t.Fatalf("unmarshal stored PipelineRun: %v", err)
	}
	if stored.Status.CompletionTime == nil {
		t.Fatal("stored PipelineRun has no CompletionTime")
	}
	return &stored
}

func scaleLoggingBackend(t *testing.T, ctx context.Context, sel string, replicas int32) {
	t.Helper()
	cs := kubernetes.NewForConfigOrDie(clientConfig(t))

	stsList, err := cs.AppsV1().StatefulSets("logging").List(ctx, metav1.ListOptions{LabelSelector: sel})
	if err != nil {
		t.Fatalf("list statefulsets: %v", err)
	}
	depList, err := cs.AppsV1().Deployments("logging").List(ctx, metav1.ListOptions{LabelSelector: sel})
	if err != nil {
		t.Fatalf("list deployments: %v", err)
	}
	if len(stsList.Items)+len(depList.Items) == 0 {
		t.Fatalf("no workload in logging matching %q", sel)
	}

	for i := range stsList.Items {
		name := stsList.Items[i].Name
		_, err := cs.AppsV1().StatefulSets("logging").UpdateScale(ctx, name, &autoscalingv1.Scale{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "logging"},
			Spec:       autoscalingv1.ScaleSpec{Replicas: replicas},
		}, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("scale sts %s: %v", name, err)
		}
	}
	for i := range depList.Items {
		name := depList.Items[i].Name
		_, err := cs.AppsV1().Deployments("logging").UpdateScale(ctx, name, &autoscalingv1.Scale{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "logging"},
			Spec:       autoscalingv1.ScaleSpec{Replicas: replicas},
		}, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("scale deploy %s: %v", name, err)
		}
	}

	if err := wait.PollUntilContextTimeout(ctx, time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		pods, err := cs.CoreV1().Pods("logging").List(ctx, metav1.ListOptions{LabelSelector: sel})
		if err != nil {
			return false, err
		}
		if replicas == 0 {
			// Terminating pods can still serve traffic. Wait until they are gone
			// so the plugin cannot reuse a KeepAlive connection to Splunk.
			return len(pods.Items) == 0, nil
		}
		ready := 0
		for _, p := range pods.Items {
			for _, c := range p.Status.Conditions {
				if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
					ready++
				}
			}
		}
		return int32(ready) >= replicas, nil
	}); err != nil {
		t.Fatalf("wait for %s replicas=%d: %v", sel, replicas, err)
	}
}
