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
	"context"
	"testing"

	resultsv1alpha2 "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"

	"strings"
	"time"

	"os"

	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
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
		if err := wait.PollImmediate(1*time.Second, 1*time.Minute, func() (done bool, err error) {
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
		logClient, err := gc.GetLog(context.Background(), &resultsv1alpha2.GetLogRequest{Name: logName})
		if err != nil {
			t.Errorf("Error getting Log Client: %v", err)
		}
		log, err := logClient.Recv()
		if err != nil {
			t.Errorf("Error getting Log: %v", err)
		}
		want := "[hello : hello] hello world!"
		if !strings.Contains(string(log.Data), want) {
			t.Errorf("Log Data inconsistent got: %s, doesn't have: %s", string(log.Data), want)
		}
	})
}
