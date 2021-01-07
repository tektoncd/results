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

// +build e2e

package e2e

import (
	"io/ioutil"
	"testing"
	"time"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	clientset "github.com/tektoncd/pipeline/pkg/client/clientset/versioned/typed/pipeline/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"
)

const (
	ns = "default"
)

func TestTaskRun(t *testing.T) {
	tr := new(v1beta1.TaskRun)
	b, err := ioutil.ReadFile("testdata/taskrun.yaml")
	if err != nil {
		t.Fatalf("ioutil.Readfile: %v", err)
	}
	if err := yaml.UnmarshalStrict(b, tr); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	c := client(t)

	// Best effort delete existing Run in case one already exists.
	_ = c.TaskRuns(ns).Delete(tr.GetName(), metav1.NewDeleteOptions(0))

	if _, err = c.TaskRuns(ns).Create(tr); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Wait for Result ID to show up.
	if err := wait.PollImmediate(1*time.Second, 10*time.Second, func() (done bool, err error) {
		tr, err := c.TaskRuns(ns).Get(tr.GetName(), metav1.GetOptions{})
		if err != nil {
			t.Logf("Get: %v", err)
			return false, nil
		}
		if r, ok := tr.GetAnnotations()["results.tekton.dev/result"]; ok {
			t.Logf("Found Result: %s", r)
			return true, nil
		}
		return false, nil
	}); err != nil {
		t.Fatalf("error waiting for Result ID: %v", err)
	}
}

func TestPipelineRun(t *testing.T) {
	pr := new(v1beta1.PipelineRun)
	b, err := ioutil.ReadFile("testdata/pipelinerun.yaml")
	if err != nil {
		t.Fatalf("ioutil.Readfile: %v", err)
	}
	if err := yaml.UnmarshalStrict(b, pr); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	c := client(t)

	// Best effort delete existing Run in case one already exists.
	_ = c.PipelineRuns(ns).Delete(pr.GetName(), metav1.NewDeleteOptions(0))

	if _, err = c.PipelineRuns(ns).Create(pr); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Wait for Result ID to show up.
	if err := wait.PollImmediate(1*time.Second, 10*time.Second, func() (done bool, err error) {
		pr, err := c.PipelineRuns(ns).Get(pr.GetName(), metav1.GetOptions{})
		if err != nil {
			t.Logf("Get: %v", err)
			return false, nil
		}
		if r, ok := pr.GetAnnotations()["results.tekton.dev/result"]; ok {
			t.Logf("Found Result: %s", r)
			return true, nil
		}
		return false, nil
	}); err != nil {
		t.Fatalf("error waiting for Result ID: %v", err)
	}
}

func client(t *testing.T) *clientset.TektonV1beta1Client {
	t.Helper()

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
	config, err := kubeconfig.ClientConfig()
	if err != nil {
		panic(err)
	}
	return clientset.NewForConfigOrDie(config)
}
