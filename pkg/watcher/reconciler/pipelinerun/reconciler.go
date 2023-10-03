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
	"fmt"

	"github.com/tektoncd/results/pkg/apis/config"
	"github.com/tektoncd/results/pkg/pipelinerunmetrics"

	pipelinev1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	pipelinev1beta1listers "github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1beta1"
	"github.com/tektoncd/results/pkg/watcher/reconciler"
	resultsannotation "github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	"github.com/tektoncd/results/pkg/watcher/reconciler/dynamic"
	"github.com/tektoncd/results/pkg/watcher/results"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	knativereconciler "knative.dev/pkg/reconciler"
)

// Reconciler represents pipelineRun watcher logic
type Reconciler struct {
	// Inline LeaderAwareFuncs to support leader election.
	knativereconciler.LeaderAwareFuncs

	resultsClient     pb.ResultsClient
	logsClient        pb.LogsClient
	pipelineRunLister pipelinev1beta1listers.PipelineRunLister
	taskRunLister     pipelinev1beta1listers.TaskRunLister
	pipelineClient    versioned.Interface
	cfg               *reconciler.Config
	metrics           *pipelinerunmetrics.Recorder
	configStore       *config.Store
}

// Check that our Reconciler is LeaderAware.
var _ knativereconciler.LeaderAware = (*Reconciler)(nil)

// Reconcile makes new watcher reconcile cycle to handle PipelineRun.
func (r *Reconciler) Reconcile(ctx context.Context, key string) error {
	logger := logging.FromContext(ctx).With(zap.String("results.tekton.dev/kind", "PipelineRun"))

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		logger.Errorf("Invalid resource key provided: %s. Skipping reconciliation.", key)
		return nil
	}

	if !r.IsLeaderFor(types.NamespacedName{Namespace: namespace, Name: name}) {
		logger.Debugf("Instance is not the leader for PipelineRun '%s/%s', skipping reconciliation.", namespace, name)
		return controller.NewSkipKey(key)
	}

	logger.Infof("Initiating reconciliation for PipelineRun '%s/%s'", namespace, name)

	pr, err := r.pipelineRunLister.PipelineRuns(namespace).Get(name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Debug("Skipping key: object is no longer available")
			return controller.NewSkipKey(key)
		}
		return fmt.Errorf("error reading PipelineRun from the indexer: %w", err)
	}

	pipelineRunClient := &dynamic.PipelineRunClient{
		PipelineRunInterface: r.pipelineClient.TektonV1beta1().PipelineRuns(namespace),
	}

	dyn := dynamic.NewDynamicReconciler(r.resultsClient, r.logsClient, pipelineRunClient, r.cfg)
	// Tell the dynamic reconciler to wait until all underlying TaskRuns are
	// ready for deletion before deleting the PipelineRun. This guarantees
	// that the TaskRuns will not be deleted before their final state being
	// properly archived into the API server.
	dyn.IsReadyForDeletionFunc = r.areAllUnderlyingTaskRunsReadyForDeletion
	dyn.AfterDeletion = func(ctx context.Context, object results.Object) error {
		pr := object.(*pipelinev1beta1.PipelineRun)
		return r.metrics.DurationAndCountDeleted(ctx, r.configStore.Load().Metrics, pr)
	}

	return dyn.Reconcile(logging.WithLogger(ctx, logger), pr)
}

func (r *Reconciler) areAllUnderlyingTaskRunsReadyForDeletion(ctx context.Context, object results.Object) (bool, error) {
	pipelineRun, ok := object.(*pipelinev1beta1.PipelineRun)
	if !ok {
		return false, fmt.Errorf("unexpected object (must not happen): want %T, but got %T", &pipelinev1beta1.PipelineRun{}, object)
	}

	logger := logging.FromContext(ctx)

	// Support both minimal and full embedded status (see the TODO comment
	// below).
	if len(pipelineRun.Status.ChildReferences) > 0 {
		for _, reference := range pipelineRun.Status.ChildReferences {
			taskRun, err := r.taskRunLister.TaskRuns(pipelineRun.Namespace).Get(reference.Name)
			if err != nil {
				if apierrors.IsNotFound(err) {
					// Let's assume that the TaskRun in
					// question is gone and therefore, we
					// can safely ignore it.
					logger.Debugf("TaskRun %s/%s associated with PipelineRun %s is no longer available. Skipping.", pipelineRun.Namespace, reference.Name, pipelineRun.Name)
					continue
				}
				return false, fmt.Errorf("error reading TaskRun from the indexer: %w", err)
			}
			if !isMarkedAsReadyForDeletion(taskRun) {
				logger.Debugf("TaskRun %s/%s associated with PipelineRun %s isn't yet ready to be deleted - the annotation %s is missing", taskRun.Namespace, taskRun.Name, pipelineRun.Name, resultsannotation.ChildReadyForDeletion)
				return false, nil
			}
		}
	} else {
		// TODO(alan-ghelardi): remove this else once we support only
		// Tekton v1 API since the full embedded status will no longer
		// be supported.
		for taskRunName := range pipelineRun.Status.TaskRuns {
			taskRun, err := r.taskRunLister.TaskRuns(pipelineRun.Namespace).Get(taskRunName)
			if err != nil {
				if apierrors.IsNotFound(err) {
					// Let's assume that the TaskRun in
					// question is gone and therefore, we
					// can safely ignore it.
					logger.Debugf("TaskRun %s/%s associated with PipelineRun %s is no longer available. Skipping.", pipelineRun.Namespace, taskRunName, pipelineRun.Name)
					continue
				}
				return false, fmt.Errorf("error reading TaskRun from the indexer: %w", err)
			}
			if !isMarkedAsReadyForDeletion(taskRun) {
				logger.Debugf("TaskRun %s/%s associated with PipelineRun %s isn't yet ready to be deleted - the annotation %s is missing", taskRun.Namespace, taskRunName, pipelineRun.Name, resultsannotation.ChildReadyForDeletion)
				return false, nil
			}
		}
	}

	return true, nil
}

func isMarkedAsReadyForDeletion(taskRun *pipelinev1beta1.TaskRun) bool {
	if taskRun.Annotations == nil {
		return false
	}
	if _, found := taskRun.Annotations[resultsannotation.ChildReadyForDeletion]; found {
		return true
	}
	return false
}
