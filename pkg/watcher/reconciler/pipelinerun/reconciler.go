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
	"time"

	"github.com/tektoncd/results/pkg/apis/config"
	"github.com/tektoncd/results/pkg/pipelinerunmetrics"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	pipelinerunreconciler "github.com/tektoncd/pipeline/pkg/client/injection/reconciler/pipeline/v1/pipelinerun"
	pipelinev1listers "github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1"
	"github.com/tektoncd/results/pkg/watcher/reconciler"
	resultsannotation "github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	"github.com/tektoncd/results/pkg/watcher/reconciler/client"
	"github.com/tektoncd/results/pkg/watcher/reconciler/dynamic"
	"github.com/tektoncd/results/pkg/watcher/results"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	knativereconciler "knative.dev/pkg/reconciler"
)

// Reconciler represents pipelineRun watcher logic
type Reconciler struct {

	// kubeClientSet allows us to talk to the k8s for core APIs
	kubeClientSet kubernetes.Interface

	resultsClient     pb.ResultsClient
	logsClient        pb.LogsClient
	pipelineRunLister pipelinev1listers.PipelineRunLister
	taskRunLister     pipelinev1listers.TaskRunLister
	pipelineClient    versioned.Interface
	cfg               *reconciler.Config
	metrics           *pipelinerunmetrics.Recorder
	configStore       *config.Store
}

// Check that our Reconciler implements pipelinerunreconciler.Interface and pipelinerunreconciler.Finalizer
var _ pipelinerunreconciler.Interface = (*Reconciler)(nil)
var _ pipelinerunreconciler.Finalizer = (*Reconciler)(nil)

// ReconcileKind makes new watcher reconcile cycle to handle PipelineRun.
func (r *Reconciler) ReconcileKind(ctx context.Context, pr *pipelinev1.PipelineRun) knativereconciler.Event {
	logger := logging.FromContext(ctx).With(zap.String("results.tekton.dev/kind", "PipelineRun"))

	logger.Infof("Initiating reconciliation for PipelineRun '%s/%s'", pr.Namespace, pr.Name)

	if r.cfg.DisableStoringIncompleteRuns {
		// Skip if pipelinerun is not done
		if !pr.IsDone() {
			logger.Debugf("pipelinerun %s/%s is not done and incomplete runs are disabled, skipping storing", pr.Namespace, pr.Name)
			return nil
		}

		// Skip if pipelinerun is already stored
		if pr.Annotations != nil && pr.Annotations[resultsannotation.Stored] == "true" {
			logger.Debugf("pipelinerun %s/%s is already stored, skipping", pr.Namespace, pr.Name)
			return nil
		}
	}

	pipelineRunClient := &client.PipelineRunClient{
		PipelineRunInterface: r.pipelineClient.TektonV1().PipelineRuns(pr.Namespace),
	}

	dyn := dynamic.NewDynamicReconciler(r.kubeClientSet, r.resultsClient, r.logsClient, pipelineRunClient, r.cfg)
	// Tell the dynamic reconciler to wait until all underlying TaskRuns are
	// ready for deletion before deleting the PipelineRun. This guarantees
	// that the TaskRuns will not be deleted before their final state being
	// properly archived into the API server.
	dyn.IsReadyForDeletionFunc = r.areAllUnderlyingTaskRunsReadyForDeletion
	dyn.AfterDeletion = func(ctx context.Context, object results.Object) error {
		pr := object.(*pipelinev1.PipelineRun)
		return r.metrics.DurationAndCountDeleted(ctx, r.configStore.Load().Metrics, pr)
	}

	return dyn.Reconcile(logging.WithLogger(ctx, logger), pr)
}

func (r *Reconciler) areAllUnderlyingTaskRunsReadyForDeletion(ctx context.Context, object results.Object) (bool, error) {
	pipelineRun, ok := object.(*pipelinev1.PipelineRun)
	if !ok {
		return false, fmt.Errorf("unexpected object (must not happen): want %T, but got %T", &pipelinev1.PipelineRun{}, object)
	}

	logger := logging.FromContext(ctx)

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
	}

	return true, nil
}

func isMarkedAsReadyForDeletion(taskRun *pipelinev1.TaskRun) bool {
	if taskRun.Annotations == nil {
		return false
	}
	if _, found := taskRun.Annotations[resultsannotation.ChildReadyForDeletion]; found {
		return true
	}
	return false
}

// FinalizeKind implements pipelinerunreconciler.Finalizer
// We utilize finalizers to ensure that we get a crack at storing every pipelinerun
// that we see flowing through the system.  If we don't add a finalizer, it could
// get cleaned up before we see the final state and store it.
func (r *Reconciler) FinalizeKind(ctx context.Context, pr *pipelinev1.PipelineRun) knativereconciler.Event {
	// Reconcile the pipelinerun to ensure that it is stored in the database
	rerr := r.ReconcileKind(ctx, pr)

	return r.finalize(ctx, pr, rerr)
}

func (r *Reconciler) finalize(ctx context.Context, pr *pipelinev1.PipelineRun, rerr error) knativereconciler.Event {
	// If logsClient isn't nil, it means we have logging storage enabled
	// and we can't use finalizers to coordinate deletion.
	if r.logsClient != nil {
		return nil
	}

	// If annotation update is disabled, we can't use finalizers to coordinate deletion.
	if r.cfg.DisableAnnotationUpdate {
		return nil
	}

	// Check to make sure the PipelineRun is finished.
	if !pr.IsDone() {
		logging.FromContext(ctx).Debugf("pipelinerun %s/%s is still running", pr.Namespace, pr.Name)
		return nil
	}

	var storeDeadline, now time.Time

	// Check if the store deadline is configured
	if r.cfg.StoreDeadline != nil {
		if pr.Status.CompletionTime == nil {
			logging.FromContext(ctx).Infof("removing finalizer without wait, no completion time set for pipelinerun %s/%s",
				pr.Namespace, pr.Name)
			return nil
		}
		now = time.Now().UTC()
		storeDeadline = pr.Status.CompletionTime.UTC().Add(*r.cfg.StoreDeadline)
		if now.After(storeDeadline) {
			logging.FromContext(ctx).Debugf("store deadline: %s now: %s, completion time: %s", storeDeadline.String(), now.String(),
				pr.Status.CompletionTime.UTC().String())
			logging.FromContext(ctx).Debugf("store deadline has passed for pipelinerun %s/%s", pr.Namespace, pr.Name)
			_, ok := pr.Annotations[resultsannotation.Stored]
			if !ok {
				logging.FromContext(ctx).Errorf("pipelinerun not stored: %s/%s, uid: %s,",
					pr.Namespace, pr.Name, pr.UID)
			}
			return nil // Proceed with deletion
		}
	}

	if pr.Annotations == nil {
		logging.FromContext(ctx).Debugf("pipelinerun %s/%s annotations are missing, now: %s, storeDeadline: %s",
			pr.Namespace, pr.Name, now.String(), storeDeadline.String())
		return controller.NewRequeueAfter(r.cfg.FinalizerRequeueInterval)
	}

	stored, ok := pr.Annotations[resultsannotation.Stored]
	if !ok {
		logging.FromContext(ctx).Debugf("stored annotation is missing on pipelinerun %s/%s, now: %s, storeDeadline: %s",
			pr.Namespace, pr.Name, now.String(), storeDeadline.String())
		return controller.NewRequeueAfter(r.cfg.FinalizerRequeueInterval)
	}
	if rerr != nil || stored != "true" {
		logging.FromContext(ctx).Debugf("stored annotation is not true on pipelinerun %s/%s, now: %s, storeDeadline: %s",
			pr.Namespace, pr.Name, now.String(), storeDeadline.String())
		return controller.NewRequeueAfter(r.cfg.FinalizerRequeueInterval)
	}

	return nil
}
