// Copyright 2020 The Tekton Authors
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

package pipelinerun

import (
	"context"

	"github.com/tektoncd/results/pkg/apis/config"
	"github.com/tektoncd/results/pkg/pipelinerunmetrics"
	"knative.dev/pkg/configmap"

	pipelineclient "github.com/tektoncd/pipeline/pkg/client/injection/client"
	pipelineruninformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1/pipelinerun"
	taskruninformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1/taskrun"
	pipelinerunreconciler "github.com/tektoncd/pipeline/pkg/client/injection/reconciler/pipeline/v1/pipelinerun"
	"github.com/tektoncd/results/pkg/watcher/logs"
	"github.com/tektoncd/results/pkg/watcher/reconciler"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
)

// NewController creates a Controller for watching PipelineRuns.
func NewController(ctx context.Context, resultsClient pb.ResultsClient, cmw configmap.Watcher) *controller.Impl {
	return NewControllerWithConfig(ctx, resultsClient, &reconciler.Config{}, cmw)
}

// NewControllerWithConfig creates a Controller for watching PipelineRuns by config.
func NewControllerWithConfig(ctx context.Context, resultsClient pb.ResultsClient, cfg *reconciler.Config, cmw configmap.Watcher) *controller.Impl {
	pipelineRunInformer := pipelineruninformer.Get(ctx)
	pipelineRunLister := pipelineRunInformer.Lister()
	logger := logging.FromContext(ctx)
	configStore := config.NewStore(logger.Named("config-store"), pipelinerunmetrics.MetricsOnStore(logger))
	configStore.WatchConfigs(cmw)

	c := &Reconciler{
		kubeClientSet:     kubeclient.Get(ctx),
		resultsClient:     resultsClient,
		logsClient:        logs.Get(ctx),
		pipelineRunLister: pipelineRunLister,
		taskRunLister:     taskruninformer.Get(ctx).Lister(),
		pipelineClient:    pipelineclient.Get(ctx),
		cfg:               cfg,
		configStore:       configStore,
		metrics:           pipelinerunmetrics.NewRecorder(),
	}

	impl := pipelinerunreconciler.NewImpl(ctx, c, func(_ *controller.Impl) controller.Options {
		return controller.Options{
			// This results pipelinerun reconciler shouldn't mutate the pipelinerun's status.
			SkipStatusUpdates: true,
			ConfigStore:       configStore,
			FinalizerName:     "results.tekton.dev/pipelinerun",
		}
	})

	_, err := pipelineRunInformer.Informer().AddEventHandler(controller.HandleAll(impl.Enqueue))
	if err != nil {
		logger.Panicf("Couldn't register PipelineRun informer event handler: %w", err)
	}

	return impl
}
