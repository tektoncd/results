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

//go:build e2e && splunk
// +build e2e,splunk

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	resultsv1alpha2 "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestSplunkPluginLogs(t *testing.T) {
	ctx := context.Background()

	gcAdmin, _ := resultsClient(t, allNamespacesAdminAccessTokenFile, nil)

	resultName, recordName, _ := createAndWaitPluginLogPipelineRun(t, ctx, "plugin-logs-splunk")
	_, _, _, logURL := pluginLogURL(resultName, recordName)

	t.Run("complete content via v1alpha3 plugin route", func(t *testing.T) {
		// Splunk search latest_time is completion + splunkForwarderDelay. Vector may
		// ship step logs late (collection _time), so poll past that window.
		got := waitForPluginLogContent(t, ctx, allNamespacesReadAccessTokenFile, logURL, logMarkers, splunkForwarderDelay+2*time.Minute)
		if got.StatusCode != http.StatusOK {
			t.Fatalf("status %d body %q", got.StatusCode, got.Body)
		}
	})

	// LogMux: getRecord error → HTTP 500 with not-found in the body.
	// rec == nil is not the live path (gorm First returns an error, not a nil record).
	t.Run("unknown record id", func(t *testing.T) {
		parent, resultID, _, _ := pluginLogURL(resultName, recordName)
		missing := strings.TrimRight(serverAddress, "/") +
			"/apis/results.tekton.dev/v1alpha3/parents/" + parent +
			"/results/" + resultID + "/logs/00000000-0000-0000-0000-000000000000"
		got := getPluginLogs(t, ctx, allNamespacesReadAccessTokenFile, missing)
		if got.StatusCode != http.StatusInternalServerError {
			t.Fatalf("status %d want 500 body %q", got.StatusCode, got.Body)
		}
		if !strings.Contains(strings.ToLower(got.Body), "record not found") &&
			!strings.Contains(strings.ToLower(got.Body), "not found") {
			t.Errorf("expected not-found contract, body %q", got.Body)
		}
	})

	t.Run("record exists with no splunk logs", func(t *testing.T) {
		fakeUID := uniquePluginRecordUID()
		res, err := gcAdmin.CreateResult(ctx, &resultsv1alpha2.CreateResultRequest{
			Parent: defaultNamespace,
			Result: &resultsv1alpha2.Result{Name: defaultNamespace + "/results/" + fakeUID},
		})
		if err != nil {
			t.Fatalf("CreateResult: %v", err)
		}
		now := time.Now().UTC()
		pr := tektonv1.PipelineRun{
			TypeMeta:   metav1.TypeMeta{APIVersion: "tekton.dev/v1", Kind: "PipelineRun"},
			ObjectMeta: metav1.ObjectMeta{UID: types.UID(fakeUID), Namespace: defaultNamespace, Name: "no-logs"},
			Status: tektonv1.PipelineRunStatus{
				PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
					StartTime:      &metav1.Time{Time: now.Add(-time.Minute)},
					CompletionTime: &metav1.Time{Time: now},
				},
			},
		}
		raw, err := json.Marshal(pr)
		if err != nil {
			t.Fatalf("marshal PipelineRun: %v", err)
		}
		recName := res.GetName() + "/records/" + fakeUID
		_, err = gcAdmin.CreateRecord(ctx, &resultsv1alpha2.CreateRecordRequest{
			Parent: res.GetName(),
			Record: &resultsv1alpha2.Record{
				Name: recName,
				Data: &resultsv1alpha2.Any{Type: "tekton.dev/v1.PipelineRun", Value: raw},
			},
		})
		if err != nil {
			t.Fatalf("CreateRecord: %v", err)
		}

		_, _, _, emptyURL := pluginLogURL(res.GetName(), recName)
		got := getPluginLogs(t, ctx, allNamespacesReadAccessTokenFile, emptyURL)
		if got.StatusCode != http.StatusOK {
			t.Fatalf("status %d want 200 body %q", got.StatusCode, got.Body)
		}
		if strings.TrimSpace(got.Body) != "" {
			t.Fatalf("want empty body for record with no splunk logs, got %q", got.Body)
		}
	})

	t.Run("backend down returns error not hang", func(t *testing.T) {
		scaleLoggingBackend(t, ctx, "app=splunk", 0)
		t.Cleanup(func() { scaleLoggingBackend(t, ctx, "app=splunk", 1) })

		// After scale-to-0 the plugin Dial timeout (20s) expires, then returns 500.
		const backendDownBudget = 45 * time.Second
		start := time.Now()
		got := getPluginLogsWithTimeout(t, ctx, allNamespacesReadAccessTokenFile, logURL, backendDownBudget)
		if time.Since(start) > backendDownBudget {
			t.Fatalf("request hung %s", time.Since(start))
		}
		if got.StatusCode != http.StatusInternalServerError {
			t.Fatalf("status %d want 500 body %q", got.StatusCode, got.Body)
		}
	})
}
