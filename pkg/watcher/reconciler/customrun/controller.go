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

// Package customrun provides the CustomRun reconciler controller.
package customrun

import (
	"context"

	pipelineclient "github.com/tektoncd/pipeline/pkg/client/injection/client"
	customruninformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1beta1/customrun"
	customrunreconciler "github.com/tektoncd/pipeline/pkg/client/injection/reconciler/pipeline/v1beta1/customrun"
	"github.com/tektoncd/results/pkg/apis/config"
	"github.com/tektoncd/results/pkg/customrunmetrics"
	"github.com/tektoncd/results/pkg/metrics"
	"github.com/tektoncd/results/pkg/watcher/reconciler"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
)

// NewController creates a Controller for watching CustomRuns.
func NewController(ctx context.Context, resultsClient pb.ResultsClient, cmw configmap.Watcher) *controller.Impl {
	return NewControllerWithConfig(ctx, resultsClient, &reconciler.Config{}, cmw)
}

// NewControllerWithConfig creates a Controller for watching CustomRuns by config.
func NewControllerWithConfig(ctx context.Context, resultsClient pb.ResultsClient, cfg *reconciler.Config, cmw configmap.Watcher) *controller.Impl {
	informer := customruninformer.Get(ctx)
	lister := informer.Lister()
	logger := logging.FromContext(ctx)
	configStore := config.NewStore(logger.Named("config-store"))
	configStore.WatchConfigs(cmw)

	// Initialize metrics once at startup
	metrics.EnsureMetricsInitialized(logger)
	customRunMetrics, err := customrunmetrics.NewRecorder(ctx)
	if err != nil {
		logger.Errorf("Failed to create customrun metrics recorder: %v. Metrics will not be recorded.", err)
	}

	c := &Reconciler{
		kubeClientSet:    kubeclient.Get(ctx),
		resultsClient:    resultsClient,
		customRunLister:  lister,
		pipelineClient:   pipelineclient.Get(ctx),
		cfg:              cfg,
		configStore:      configStore,
		metrics:          metrics.NewRecorder(),
		customRunMetrics: customRunMetrics,
	}

	impl := customrunreconciler.NewImpl(ctx, c, func(_ *controller.Impl) controller.Options {
		return controller.Options{
			// This results customrun reconciler shouldn't mutate the customrun's status.
			SkipStatusUpdates: true,
			ConfigStore:       configStore,
			FinalizerName:     "results.tekton.dev/customrun",
		}
	})

	_, err = informer.Informer().AddEventHandler(controller.HandleAll(impl.Enqueue))
	if err != nil {
		logger.Panicf("Couldn't register CustomRun informer event handler: %w", err)
	}

	return impl
}
