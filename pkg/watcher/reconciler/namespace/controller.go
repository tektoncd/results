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

// Package namespace provides a controller that cleans up Results API data when namespaces are deleted.
package namespace

import (
	"context"

	leaderelection "github.com/tektoncd/results/pkg/watcher/reconciler/leaderelection"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	factory "knative.dev/pkg/client/injection/kube/informers/factory"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
)

// NewController creates a controller that watches for namespace deletions
// and cleans up associated Results API data.
func NewController(ctx context.Context, resultsClient pb.ResultsClient) *controller.Impl {
	logger := logging.FromContext(ctx)

	informerFactory := factory.Get(ctx)
	nsInformer := informerFactory.Core().V1().Namespaces()

	r := &Reconciler{
		LeaderAwareFuncs: leaderelection.NewLeaderAwareFuncs(nsInformer.Lister().List),
		resultsClient:    resultsClient,
		namespaceLister:  nsInformer.Lister(),
	}

	impl := controller.NewContext(ctx, r, controller.ControllerOptions{
		WorkQueueName: "NamespaceCleanup",
		Logger:        logger.Desugar().Sugar(),
	})

	_, err := nsInformer.Informer().AddEventHandler(controller.HandleAll(impl.Enqueue))
	if err != nil {
		logger.Panicf("Couldn't register Namespace informer event handler: %w", err)
	}

	informerFactory.Start(ctx.Done())
	informerFactory.WaitForCacheSync(ctx.Done())

	return impl
}
