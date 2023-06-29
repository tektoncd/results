/*
Copyright 2019 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	test "github.com/tektoncd/pipeline/pkg/reconciler/testing"
	"github.com/tektoncd/pipeline/test/diff"
	"github.com/tektoncd/results/pkg/apis/config"
	corev1 "k8s.io/api/core/v1"
	logtesting "knative.dev/pkg/logging/testing"
)

func TestStoreLoadWithContext(t *testing.T) {
	metricsConfig := test.ConfigMapFromTestFile(t, "config-observability")

	metrics, _ := config.NewMetricsFromConfigMap(metricsConfig)

	expected := &config.Config{
		Metrics: metrics,
	}

	store := config.NewStore(logtesting.TestLogger(t))
	store.OnConfigChanged(metricsConfig)

	cfg := config.FromContext(store.ToContext(context.Background()))

	if d := cmp.Diff(cfg, expected); d != "" {
		t.Errorf("Unexpected config %s", diff.PrintWantGot(d))
	}
}

func TestStoreLoadWithContext_Empty(t *testing.T) {
	metrics, _ := config.NewMetricsFromConfigMap(&corev1.ConfigMap{Data: map[string]string{}})

	want := &config.Config{
		Metrics: metrics,
	}

	store := config.NewStore(logtesting.TestLogger(t))

	got := config.FromContext(store.ToContext(context.Background()))

	if d := cmp.Diff(want, got); d != "" {
		t.Errorf("Unexpected config %s", diff.PrintWantGot(d))
	}
}
