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

package taskrun

import (
	"context"

	pipelineclient "github.com/tektoncd/pipeline/pkg/client/injection/client"
	taskruninformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1beta1/taskrun"
	"github.com/tektoncd/results/pkg/watcher/reconciler"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"k8s.io/client-go/tools/cache"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
)

// NewController creates a Controller for watching TaskRuns.
func NewController(ctx context.Context, client pb.ResultsClient) *controller.Impl {
	return NewControllerWithConfig(ctx, client, &reconciler.Config{})
}

func NewControllerWithConfig(ctx context.Context, client pb.ResultsClient, cfg *reconciler.Config) *controller.Impl {
	informer := taskruninformer.Get(ctx)

	c := &Reconciler{
		client:         client,
		lister:         informer.Lister(),
		pipelineClient: pipelineclient.Get(ctx),
		cfg:            cfg,
	}

	impl := controller.NewContext(ctx, c, controller.ControllerOptions{
		Logger:        logging.FromContext(ctx),
		WorkQueueName: "TaskRuns",
	})
	c.enqueue = impl.EnqueueAfter

	informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    impl.Enqueue,
		UpdateFunc: controller.PassNew(impl.Enqueue),
	})

	return impl
}
