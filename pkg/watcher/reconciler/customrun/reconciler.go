/*
Copyright 2026 The Tekton Authors

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

package customrun

import (
	"context"
	"fmt"
	"time"

	"github.com/tektoncd/results/pkg/apis/config"
	"github.com/tektoncd/results/pkg/customrunmetrics"
	"github.com/tektoncd/results/pkg/metrics"

	pipelinev1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	customrunreconciler "github.com/tektoncd/pipeline/pkg/client/injection/reconciler/pipeline/v1beta1/customrun"
	v1beta1 "github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1beta1"
	"github.com/tektoncd/results/pkg/watcher/reconciler"

	resultsannotation "github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	"github.com/tektoncd/results/pkg/watcher/reconciler/client"
	"github.com/tektoncd/results/pkg/watcher/reconciler/dynamic"
	"github.com/tektoncd/results/pkg/watcher/results"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	knativereconciler "knative.dev/pkg/reconciler"
)

// Reconciler represents customRun watcher logic
type Reconciler struct {
	// kubeClientSet allows us to talk to the k8s for core APIs
	kubeClientSet kubernetes.Interface

	resultsClient    pb.ResultsClient
	customRunLister  v1beta1.CustomRunLister
	pipelineClient   versioned.Interface
	cfg              *reconciler.Config
	metrics          *metrics.Recorder
	customRunMetrics *customrunmetrics.Recorder
	configStore      *config.Store
}

// Check that our Reconciler implements customrunreconciler.Interface and customrunreconciler.Finalizer
var _ customrunreconciler.Interface = (*Reconciler)(nil)
var _ customrunreconciler.Finalizer = (*Reconciler)(nil)

// ReconcileKind makes new watcher reconcile cycle to handle CustomRun.
func (r *Reconciler) ReconcileKind(ctx context.Context, cr *pipelinev1beta1.CustomRun) knativereconciler.Event {
	logger := logging.FromContext(ctx).With(zap.String("results.tekton.dev/kind", "CustomRun"))

	if r.cfg.DisableStoringIncompleteRuns {
		// Skip if customrun is not done
		if !cr.IsDone() {
			logger.Debugf("customrun %s/%s is not done and incomplete runs are disabled, skipping storing", cr.Namespace, cr.Name)
			return nil
		}

		// Skip if customrun is already stored
		if cr.Annotations != nil && cr.Annotations[resultsannotation.Stored] == "true" {
			logger.Debugf("customrun %s/%s is already stored, skipping", cr.Namespace, cr.Name)
			return nil
		}
	}

	customRunClient := &client.CustomRunClient{
		CustomRunInterface: r.pipelineClient.TektonV1beta1().CustomRuns(cr.Namespace),
	}

	// CustomRuns don't have logs (executed by custom controllers, not pods)
	// so logsClient is nil
	dyn := dynamic.NewDynamicReconciler(r.kubeClientSet, r.resultsClient, nil, customRunClient, r.cfg)
	dyn.AfterDeletion = func(ctx context.Context, object results.Object) error {
		cr, ok := object.(*pipelinev1beta1.CustomRun)
		if !ok {
			return fmt.Errorf("expected CustomRun, got %T", object)
		}
		if err := r.customRunMetrics.DurationAndCountDeleted(ctx, r.configStore.Load().Metrics, cr); err != nil {
			// Log but don't fail reconciliation for metrics issues
			logging.FromContext(ctx).Warnf("Failed to record customrun deletion metrics: %v", err)
		}
		return nil
	}
	dyn.AfterStorage = func(ctx context.Context, o results.Object, _ bool) error {
		cr, ok := o.(*pipelinev1beta1.CustomRun)
		if !ok {
			return fmt.Errorf("expected CustomRun, got %T", o)
		}
		return r.metrics.RecordStorageLatency(ctx, cr)
	}
	return dyn.Reconcile(logging.WithLogger(ctx, logger), cr)
}

// FinalizeKind handles customrun finalization
// We utilize finalizers to ensure that we get a crack at storing every customrun
// that we see flowing through the system.  If we don't add a finalizer, it could
// get cleaned up before we see the final state and store it.
func (r *Reconciler) FinalizeKind(ctx context.Context, cr *pipelinev1beta1.CustomRun) knativereconciler.Event {
	// Reconcile the customrun to ensure that it is stored in the database
	rerr := r.ReconcileKind(ctx, cr)
	if rerr != nil {
		// Keep requeue semantics in finalize() while ensuring this reconcile error is always visible.
		logging.FromContext(ctx).Warnw("reconcile during customrun finalization returned error",
			zap.Error(rerr))
	}

	return r.finalize(ctx, cr, rerr)
}

func (r *Reconciler) finalize(ctx context.Context, cr *pipelinev1beta1.CustomRun, rerr error) knativereconciler.Event {
	// If annotation update is disabled, we can't use finalizers to coordinate deletion.
	if r.cfg.DisableAnnotationUpdate {
		return nil
	}

	// Check the CustomRun has finished.
	if !cr.IsDone() {
		logging.FromContext(ctx).Debugf("customrun %s/%s is still running", cr.Namespace, cr.Name)
		return nil
	}

	now := time.Now().UTC()

	var storeDeadline time.Time

	// Check if the store deadline is configured
	if r.cfg.StoreDeadline != nil {
		if cr.Status.CompletionTime == nil {
			logging.FromContext(ctx).Infof("removing finalizer without wait, no completion time set for customrun %s/%s",
				cr.Namespace, cr.Name)
			return nil
		}
		storeDeadline = cr.Status.CompletionTime.UTC().Add(*r.cfg.StoreDeadline)
		if now.After(storeDeadline) {
			logging.FromContext(ctx).Debugf("store deadline: %s now: %s, completion time: %s", storeDeadline.String(), now.String(),
				cr.Status.CompletionTime.UTC().String())
			logging.FromContext(ctx).Debugf("store deadline has passed for customrun %s/%s", cr.Namespace, cr.Name)
			_, ok := cr.Annotations[resultsannotation.Stored]
			if !ok {
				logging.FromContext(ctx).Errorf("customrun not stored: %s/%s, uid: %s,",
					cr.Namespace, cr.Name, cr.UID)
				if err := metrics.CountRunNotStored(ctx, cr.Namespace, "CustomRun"); err != nil {
					logging.FromContext(ctx).Errorf("error counting CustomRun as not stored: %w", err)
				}
			}
			return nil // Proceed with deletion
		}
	}

	if cr.Annotations == nil {
		logging.FromContext(ctx).Debugf("customrun %s/%s annotations are missing, now: %s, storeDeadline: %s",
			cr.Namespace, cr.Name, now.String(), storeDeadline.String())
		return controller.NewRequeueAfter(r.cfg.FinalizerRequeueInterval)
	}

	stored, ok := cr.Annotations[resultsannotation.Stored]
	if !ok {
		logging.FromContext(ctx).Debugf("stored annotation is missing on customrun %s/%s, now: %s, storeDeadline: %s",
			cr.Namespace, cr.Name, now.String(), storeDeadline.String())
		return controller.NewRequeueAfter(r.cfg.FinalizerRequeueInterval)
	}
	if rerr != nil {
		return controller.NewRequeueAfter(r.cfg.FinalizerRequeueInterval)
	}
	if stored != "true" {
		logging.FromContext(ctx).Debugf("stored annotation is not true on customrun %s/%s, now: %s, storeDeadline: %s",
			cr.Namespace, cr.Name, now.String(), storeDeadline.String())
		return controller.NewRequeueAfter(r.cfg.FinalizerRequeueInterval)
	}

	return nil
}
