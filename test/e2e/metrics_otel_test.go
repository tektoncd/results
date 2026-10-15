//go:build e2e

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

package e2e

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/yaml"
)

const (
	watcherMetricsPort = "9090"
	resultsNamespace   = "tekton-pipelines"
)

func scrapeWatcherMetrics(ctx context.Context, t *testing.T) map[string]*dto.MetricFamily {
	t.Helper()

	kubeClient := kubernetes.NewForConfigOrDie(clientConfig(t))

	pods, err := kubeClient.CoreV1().Pods(resultsNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=tekton-results-watcher",
	})
	if err != nil {
		t.Fatalf("Failed to list watcher pods: %v", err)
	}

	var podName string
	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		allReady := true
		for _, cs := range pod.Status.ContainerStatuses {
			if !cs.Ready {
				allReady = false
				break
			}
		}
		if allReady {
			podName = pod.Name
			break
		}
	}
	if podName == "" {
		t.Fatalf("No Running/Ready tekton-results-watcher pod found in namespace %s", resultsNamespace)
	}

	result := kubeClient.
		CoreV1().
		RESTClient().
		Get().
		Resource("pods").
		Name(podName + ":" + watcherMetricsPort).
		Namespace(resultsNamespace).
		SubResource("proxy").
		Suffix("metrics").
		Do(ctx)

	body, err := result.Raw()
	if err != nil {
		t.Fatalf("Failed to scrape metrics from watcher pod %s: %v", podName, err)
	}

	parser := expfmt.NewTextParser(model.LegacyValidation)
	families, err := parser.TextToMetricFamilies(strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("Failed to parse metrics: %v", err)
	}
	return families
}

func waitForWatcherMetric(ctx context.Context, t *testing.T, metricName string, timeout time.Duration) map[string]*dto.MetricFamily {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		families := scrapeWatcherMetrics(ctx, t)
		if _, ok := families[metricName]; ok {
			return families
		}
		select {
		case <-ctx.Done():
			t.Fatalf("Timed out waiting for metric %q to appear (waited %v): %v", metricName, timeout, ctx.Err())
			return nil
		case <-time.After(5 * time.Second):
		}
	}
}

// TestOTelMetrics verifies the watcher exposes OTel infrastructure metrics.
func TestOTelMetrics(t *testing.T) {
	ctx := context.Background()

	t.Log("Waiting for kn_workqueue_adds_total to appear on watcher pod")
	families := waitForWatcherMetric(ctx, t, "kn_workqueue_adds_total", 2*time.Minute)
	t.Logf("Scraped %d metric families from tekton-results-watcher", len(families))

	tests := []struct {
		name   string
		prefix string
		errMsg string
	}{
		{
			name:   "Renames/workqueue_uses_kn_prefix",
			prefix: "kn_workqueue_",
			errMsg: "Expected at least one kn_workqueue_* metric, found none",
		},
		{
			name:   "Renames/go_runtime_uses_standard_prefix",
			prefix: "go_",
			errMsg: "Expected standard go_* runtime metrics, found none",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := false
			for name := range families {
				if strings.HasPrefix(name, tt.prefix) {
					found = true
					break
				}
			}
			if !found {
				t.Error(tt.errMsg)
			}
		})
	}

	// Old OC metric names must be absent.
	// TODO: Remove in a future release once no OC-based release is supported.
	for name := range families {
		if strings.HasPrefix(name, "tekton_results_") {
			t.Errorf("Old OC metric %q still present; expected removal after OTel migration", name)
		}
	}
}

func taskRunFromTestdata(t *testing.T) *tektonv1.TaskRun {
	t.Helper()
	b, err := os.ReadFile("testdata/taskrun.yaml")
	if err != nil {
		t.Fatalf("Failed to read testdata/taskrun.yaml: %v", err)
	}
	tr := new(tektonv1.TaskRun)
	if err := yaml.UnmarshalStrict(b, tr); err != nil {
		t.Fatalf("Failed to unmarshal TaskRun: %v", err)
	}
	return tr
}

func histogramSampleCount(families map[string]*dto.MetricFamily, name string) uint64 {
	fam, ok := families[name]
	if !ok {
		return 0
	}
	var total uint64
	for _, m := range fam.GetMetric() {
		if h := m.GetHistogram(); h != nil {
			total += h.GetSampleCount()
		}
	}
	return total
}

// TestOTelMetricsWatcherAfterRun creates a TaskRun and asserts that
// watcher_run_storage_latency_seconds increments by exactly 1.
func TestOTelMetricsWatcherAfterRun(t *testing.T) {
	ctx := context.Background()

	if _, err := os.Stat(allNamespacesReadAccessTokenFile); err != nil {
		t.Skipf("Results API tokens not found, skipping: %v", err)
	}

	baseline := waitForWatcherMetric(ctx, t, "kn_workqueue_adds_total", 30*time.Second)
	baselineCount := histogramSampleCount(baseline, "watcher_run_storage_latency_seconds")

	tc := tektonClient(t)
	_ = tc.TaskRuns(defaultNamespace).Delete(ctx, "hello", metav1.DeleteOptions{})

	tr := taskRunFromTestdata(t)
	tr, err := tc.TaskRuns(defaultNamespace).Create(ctx, tr, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create TaskRun: %v", err)
	}
	t.Cleanup(func() {
		_ = tc.TaskRuns(defaultNamespace).Delete(ctx, tr.GetName(), metav1.DeleteOptions{})
	})

	t.Log("Waiting for TaskRun to complete")
	if err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		current, err := tc.TaskRuns(defaultNamespace).Get(ctx, tr.GetName(), metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		cond := current.Status.GetCondition(apis.ConditionSucceeded)
		return cond != nil && cond.IsTrue(), nil
	}); err != nil {
		t.Fatalf("TaskRun did not complete successfully: %v", err)
	}

	t.Log("TaskRun completed, waiting for watcher to record storage latency")
	var after map[string]*dto.MetricFamily
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		after = scrapeWatcherMetrics(ctx, t)
		return histogramSampleCount(after, "watcher_run_storage_latency_seconds") > baselineCount, nil
	}); err != nil {
		t.Fatalf("watcher_run_storage_latency_seconds never incremented (baseline=%d): %v", baselineCount, err)
	}

	delta := histogramSampleCount(after, "watcher_run_storage_latency_seconds") - baselineCount
	if delta != 1 {
		t.Errorf("watcher_run_storage_latency_seconds delta = %d, want exactly 1", delta)
	}
	t.Logf("watcher_run_storage_latency_seconds delta: %d", delta)
}
