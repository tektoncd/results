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

package dynamic

import (
	"context"
	"fmt"

	"github.com/jonboulle/clockwork"
	"github.com/tektoncd/results/pkg/watcher/reconciler"
	"github.com/tektoncd/results/pkg/watcher/results"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/injection/clients/dynamicclient"
	"knative.dev/pkg/logging"
)

var (
	clock = clockwork.NewRealClock()
)

// NewController creates a Controller for watching TaskRuns.
func NewController(ctx context.Context, client pb.ResultsClient, gvr schema.GroupVersionResource, informer cache.SharedIndexInformer) *controller.Impl {
	return NewControllerWithConfig(ctx, client, gvr, informer, &reconciler.Config{})
}

func NewControllerWithConfig(ctx context.Context, client pb.ResultsClient, gvr schema.GroupVersionResource, informer cache.SharedIndexInformer, cfg *reconciler.Config) *controller.Impl {
	logger := logging.FromContext(ctx)

	c := &Reconciler{
		client:    results.NewClient(client),
		clientset: dynamicclient.Get(ctx),
		cfg:       cfg,
		gvr:       gvr,
	}
	impl := controller.NewImpl(c, logger, fmt.Sprintf("DynamicResultsWatcher_%s", gvr.String()))
	c.enqueue = impl.EnqueueAfter

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    impl.Enqueue,
		UpdateFunc: controller.PassNew(impl.Enqueue),
	})

	return impl
}
