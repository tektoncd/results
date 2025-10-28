// Copyright 2021 The Tekton Authors
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

//go:build e2e && gcs
// +build e2e,gcs

package e2e

import (
	"bytes"
	"context"
	"io"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	resultsv1alpha2 "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/genproto/googleapis/api/httpbody"

	"strings"
	"time"

	"os"

	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/yaml"
)

func TestGCSLog(t *testing.T) {
	ctx := context.Background()
	pr := new(tektonv1beta1.PipelineRun)
	b, err := os.ReadFile("testdata/pipelinerungcs.yaml")
	if err != nil {
		t.Fatalf("Error reading file: %v", err)
	}
	if err := yaml.UnmarshalStrict(b, pr); err != nil {
		t.Fatalf("Error unmarshalling: %v", err)
	}

	tc := tektonClient(t)

	deletePolicy := metav1.DeletePropagationForeground
	// Best effort delete existing Run in case one already exists.
	_ = tc.PipelineRuns(defaultNamespace).Delete(ctx, pr.GetName(), metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	})

	if _, err = tc.PipelineRuns(defaultNamespace).Create(ctx, pr, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Error creating PipelineRun: %v", err)
	}

	gc, _ := resultsClient(t, allNamespacesReadAccessTokenFile, nil)

	var logName string

	t.Run("check log annotation", func(t *testing.T) {
		// Wait for Result ID to show up.
		if err := wait.PollUntilContextTimeout(ctx, 1*time.Second, 1*time.Minute, true, func(ctx context.Context) (done bool, err error) {
			pr, err := tc.PipelineRuns(defaultNamespace).Get(ctx, pr.GetName(), metav1.GetOptions{})
			if err != nil {
				t.Fatalf("Error getting PipelineRun: %v", err)
			}
			var logAnnotation bool
			logName, logAnnotation = pr.GetAnnotations()["results.tekton.dev/log"]
			if logAnnotation {
				return true, nil
			}
			return false, nil
		}); err != nil {
			t.Log("dumping watcher logs")
			podLogs(t, "tekton-pipelines", "watcher")
			t.Log("dumping api logs")
			podLogs(t, "tekton-pipelines", "api")
			t.Fatalf("Error waiting for PipelineRun creation: %v", err)
		}
	})

	t.Run("check log", func(t *testing.T) {
		if logName == "" {
			t.Skip("log name not found")
		}
		_, err = gc.GetLog(context.Background(), &resultsv1alpha2.GetLogRequest{Name: logName})
		if err != nil {
			t.Errorf("Error getting Log: %v", err)
		}
	})

	t.Run("log data consistency", func(t *testing.T) {
		if logName == "" {
			t.Skip("log name not found")
		}
		if err := wait.PollUntilContextTimeout(ctx, 1*time.Second, 10*time.Second, true, func(ctx context.Context) (done bool, err error) {
			logClient, err := gc.GetLog(ctx, &resultsv1alpha2.GetLogRequest{Name: logName})
			if err != nil {
				t.Logf("Error getting Log Client: %v", err)
				return false, nil
			}
			var log *httpbody.HttpBody
			var cerr error
			log, cerr = logClient.Recv()
			if cerr != nil {
				t.Logf("Error getting Log for %s: %v", logName, cerr)
				return false, nil
			}
			want := "[hello : hello] hello world!"
			if log == nil {
				t.Logf("Nil return from logClient.Recv()")
				return false, nil
			}
			if !strings.Contains(string(log.Data), want) {
				t.Logf("Log Data inconsistent for %s got: %s, doesn't have: %s", logName, string(log.Data), want)
				return false, nil
			}
			return true, nil

		}); err != nil {
			t.Log("dumping watcher logs")
			podLogs(t, "tekton-pipelines", "watcher")
			t.Log("dumping api logs")
			podLogs(t, "tekton-pipelines", "api")
			t.Fatalf("Error waiting for check log: %v", err)
		}
	})
}

func podLogs(t *testing.T, ns string, name string) {
	t.Logf("getting pod logs for the pattern %s", name)
	clientset := kubernetes.NewForConfigOrDie(clientConfig(t))
	ctx := context.Background()
	list, err := clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Errorf("pod list error %s", err)
	}
	for _, pod := range list.Items {
		if strings.Contains(pod.Name, name) {
			t.Logf("found pod %s matcher pattern %s", pod.Name, name)
			for _, c := range pod.Spec.Containers {
				containerLogs(t, ctx, ns, pod.Name, c.Name)
			}
			break
		}
	}
}

func containerLogs(t *testing.T, ctx context.Context, ns, podName, containerName string) {
	podLogOpts := corev1.PodLogOptions{}
	podLogOpts.Container = containerName
	t.Logf("print container %s from pod %s:", containerName, podName)
	clientset := kubernetes.NewForConfigOrDie(clientConfig(t))
	req := clientset.CoreV1().Pods(ns).GetLogs(podName, &podLogOpts)
	logs, err := req.Stream(ctx)
	if err != nil {
		t.Errorf("error streaming pod logs %s", err.Error())
		return
	}
	defer logs.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, logs)
	if err != nil {
		t.Errorf("error copying pod logs %s", err.Error())
		return
	}
	str := buf.String()
	t.Logf("%s", str)

}
