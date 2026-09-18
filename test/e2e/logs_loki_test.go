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

//go:build e2e && loki
// +build e2e,loki

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	resultsv1alpha2 "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

func TestLokiPluginLogs(t *testing.T) {
	ctx := context.Background()
	since := time.Now()

	gc, _ := resultsClient(t, allNamespacesReadAccessTokenFile, nil)
	gcAdmin, _ := resultsClient(t, allNamespacesAdminAccessTokenFile, nil)

	resultName, recordName, _ := createAndWaitPluginLogPipelineRun(t, ctx, "plugin-logs-loki")
	_, _, _, logURL := pluginLogURL(resultName, recordName)

	t.Run("complete content via v1alpha3 plugin route", func(t *testing.T) {
		got := waitForPluginLogContent(t, ctx, allNamespacesReadAccessTokenFile, logURL, logMarkers, 3*time.Minute)
		if got.StatusCode != http.StatusOK {
			t.Fatalf("status %d body %q", got.StatusCode, got.Body)
		}
		for _, step := range []string{"step-alpha", "step-beta", "step-gamma"} {
			if !strings.Contains(got.Body, "container-"+step) {
				t.Errorf("missing container identity %s in %q", step, got.Body)
			}
		}
	})

	t.Run("forwarder delay window is applied", func(t *testing.T) {
		stored := storedPipelineRun(t, ctx, gc, recordName)
		if stored.Status.CompletionTime == nil {
			t.Fatal("PipelineRun has no completion time")
		}

		wantEnd := stored.Status.CompletionTime.UTC().Add(lokiForwarderDelay).Unix()
		apiLogs := resultsAPIPodLogs(t, ctx, since)
		var gotEnd int64
		found := false
		for _, line := range strings.Split(apiLogs, "\n") {
			if !strings.Contains(line, "loki request url:") {
				continue
			}
			i := strings.Index(line, "http")
			if i < 0 {
				continue
			}
			u, err := url.Parse(strings.Trim(line[i:], `"`))
			if err != nil {
				t.Fatalf("parse loki url: %v", err)
			}
			gotEnd, err = strconv.ParseInt(u.Query().Get("end"), 10, 64)
			if err != nil {
				t.Fatalf("end param: %v url=%s", err, u)
			}
			found = true
			if gotEnd != wantEnd {
				t.Errorf("loki end=%d want completion+delay=%d (completion=%s delay=%s) url=%s",
					gotEnd, wantEnd, stored.Status.CompletionTime.UTC().Format(time.RFC3339), lokiForwarderDelay, u)
			}
		}
		if !found {
			t.Fatalf("expected debug loki request url in API logs:\n%s", apiLogs)
		}
	})

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

	t.Run("record exists with no loki logs", func(t *testing.T) {
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
		raw, _ := json.Marshal(pr)
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
			t.Fatalf("want empty body for record with no loki logs, got %q", got.Body)
		}
	})

	t.Run("RBAC unauthorized token", func(t *testing.T) {
		got := getPluginLogs(t, ctx, singleNamespaceReadAccessTokenFile,
			strings.TrimRight(serverAddress, "/")+
				"/apis/results.tekton.dev/v1alpha3/parents/tekton/results/x/logs/y")
		if got.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status %d want 401 body %q", got.StatusCode, got.Body)
		}
	})

	t.Run("RBAC invalid token", func(t *testing.T) {
		invalid := pathJoinToken(t)
		got := getPluginLogs(t, ctx, invalid, logURL)
		if got.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status %d want 401 body %q", got.StatusCode, got.Body)
		}
	})

	t.Run("log line order is chronological", func(t *testing.T) {
		got := waitForPluginLogContent(t, ctx, allNamespacesReadAccessTokenFile, logURL, []string{
			"PLUGIN_LOG_E2E_ORDER_1",
			"PLUGIN_LOG_E2E_ORDER_2",
			"PLUGIN_LOG_E2E_ORDER_3",
		}, 3*time.Minute)
		i1 := strings.Index(got.Body, "PLUGIN_LOG_E2E_ORDER_1")
		i2 := strings.Index(got.Body, "PLUGIN_LOG_E2E_ORDER_2")
		i3 := strings.Index(got.Body, "PLUGIN_LOG_E2E_ORDER_3")
		if i1 < 0 || i2 < 0 || i3 < 0 {
			t.Fatalf("missing order markers in %q", got.Body)
		}
		if !(i1 < i2 && i2 < i3) {
			t.Fatalf("order want 1<2<3, idx %d %d %d body %q", i1, i2, i3, got.Body)
		}
	})

	t.Run("backend down returns error not hang", func(t *testing.T) {
		cs := kubernetes.NewForConfigOrDie(clientConfig(t))
		// Helm singleBinary is typically a StatefulSet named "loki".
		scale := func(n int32) {
			_, err := cs.AppsV1().StatefulSets("logging").UpdateScale(ctx, "loki",
				&autoscalingv1.Scale{ObjectMeta: metav1.ObjectMeta{Name: "loki", Namespace: "logging"}, Spec: autoscalingv1.ScaleSpec{Replicas: n}},
				metav1.UpdateOptions{})
			if err != nil {
				t.Fatalf("scale loki: %v", err)
			}
		}
		scale(0)
		t.Cleanup(func() { scale(1) })
		time.Sleep(2 * time.Second)

		start := time.Now()
		got := getPluginLogs(t, ctx, allNamespacesReadAccessTokenFile, logURL)
		if time.Since(start) > 15*time.Second {
			t.Fatalf("request hung %s", time.Since(start))
		}
		if got.StatusCode != http.StatusInternalServerError {
			t.Fatalf("status %d want 500 body %q", got.StatusCode, got.Body)
		}
	})
}

func pathJoinToken(t *testing.T) string {
	t.Helper()
	p := defaultTokenPath + "/invalid-token-loki"
	if err := os.WriteFile(p, []byte("not-a-token"), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
