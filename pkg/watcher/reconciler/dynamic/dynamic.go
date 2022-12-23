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
	"time"

	"github.com/jonboulle/clockwork"
	pipelinev1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/results/pkg/watcher/convert"
	"github.com/tektoncd/results/pkg/watcher/reconciler"
	"github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	"github.com/tektoncd/results/pkg/watcher/results"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
)

var (
	clock = clockwork.NewRealClock()
)

// Reconciler implements common reconciler behavior across different Tekton Run
// Object types.
type Reconciler struct {
	resultsClient *results.Client
	objectClient  ObjectClient
	cfg           *reconciler.Config
}

// NewDynamicReconciler creates a new dynamic Reconciler.
func NewDynamicReconciler(rc pb.ResultsClient, oc ObjectClient, cfg *reconciler.Config) *Reconciler {
	return &Reconciler{
		resultsClient: &results.Client{ResultsClient: rc},
		objectClient:  oc,
		cfg:           cfg,
	}
}

// Reconcile handles result/record uploading for the given Run object.
// If enabled, the object may be deleted upon successful result upload.
func (r *Reconciler) Reconcile(ctx context.Context, o results.Object) error {
	logger := logging.FromContext(ctx)

	if o.GetObjectKind().GroupVersionKind().Empty() {
		gvk, err := convert.InferGVK(o)
		if err != nil {
			return err
		}
		o.GetObjectKind().SetGroupVersionKind(gvk)
		logger.Debugf("Post SetGroupVersionKind: %s", o.GetObjectKind().GroupVersionKind().String())
	}

	// Upsert record.
	startTime := time.Now()
	result, record, err := r.resultsClient.Put(ctx, o)
	timeTakenField := zap.Int64("results.tekton.dev/time-taken-ms", time.Since(startTime).Milliseconds())

	if err != nil {
		logger.Debugw("Error upserting record", zap.Error(err), timeTakenField)
		return fmt.Errorf("error upserting record: %w", err)
	}

	logger = logger.With(zap.String("results.tekton.dev/result", result.Name),
		zap.String("results.tekton.dev/record", record.Name))
	logger.Debugw("Record has been successfully upserted into API server", timeTakenField)

	if err := r.addResultsAnnotations(logging.WithLogger(ctx, logger), o, result, record); err != nil {
		return fmt.Errorf("error adding Result annotations to the object: %w", err)
	}

	return r.deleteUponCompletion(logging.WithLogger(ctx, logger), o)
}

// addResultsAnnotations adds Results annotations to the object in question if
// annotation patching is enabled.
func (r *Reconciler) addResultsAnnotations(ctx context.Context, o results.Object, result *pb.Result, record *pb.Record) error {
	logger := logging.FromContext(ctx)

	objectAnnotations := o.GetAnnotations()
	if r.cfg.GetDisableAnnotationUpdate() {
		logger.Debug("Skipping CRD annotation patch: annotation update is disabled")
	} else if result.GetName() == objectAnnotations[annotation.Result] && record.GetName() == objectAnnotations[annotation.Record] {
		logger.Debug("Skipping CRD annotation patch: Result annotations are already set")
	} else {
		// Update object with Result Annotations.
		patch, err := annotation.Add(result.GetName(), record.GetName())
		if err != nil {
			logger.Errorw("Error adding Result annotations", zap.Error(err))
			return err
		}
		if err := r.objectClient.Patch(ctx, o.GetName(), types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
			logger.Errorw("Error patching object", zap.Error(err))
			return err
		}
	}
	return nil
}

// deleteUponCompletion deletes the object in question when the following
// conditions are met:
// * The resource deletion is enabled in the config (the grace period is greater
// than 0).
// * The object is done and it isn't owned by other object.
// * The configured grace period has elapsed since the object's completion.
func (r *Reconciler) deleteUponCompletion(ctx context.Context, o results.Object) error {
	logger := logging.FromContext(ctx)

	gracePeriod := r.cfg.GetCompletedResourceGracePeriod()
	logger = logger.With(zap.Duration("results.tekton.dev/gracePeriod", gracePeriod))
	if gracePeriod == 0 {
		logger.Info("Skipping resource deletion: deletion is disabled")
		return nil
	}

	if !isDone(o) {
		logger.Debug("Skipping resource deletion: object is not done yet")
		return nil
	}

	if ownerReferences := o.GetOwnerReferences(); len(ownerReferences) > 0 {
		logger.Debugw("Resource is owned by another object, deferring deletion to parent resource(s)", zap.Any("results.tekton.dev/ownerReferences", ownerReferences))
		return nil
	}

	completionTime, err := getCompletionTime(o)
	if err != nil {
		return err
	}

	// This isn't probable since the object is done, but defensive
	// programming never hurts.
	if completionTime == nil {
		logger.Debug("Object's completion time isn't set yet - requeuing to process later")
		return controller.NewRequeueAfter(gracePeriod)
	}

	if timeSinceCompletion := clock.Since(*completionTime); timeSinceCompletion < gracePeriod {
		requeueAfter := gracePeriod - timeSinceCompletion
		logger.Debugw("Object is not ready for deletion yet - requeuing to process later", zap.Duration("results.tekton.dev/requeueAfter", requeueAfter))
		return controller.NewRequeueAfter(requeueAfter)
	}

	logger.Infow("Deleting object", zap.String("results.tekton.dev/uid", string(o.GetUID())),
		zap.Int64("results.tekton.dev/time-taken-seconds", int64(time.Since(*completionTime).Seconds())))
	if err := r.objectClient.Delete(ctx, o.GetName(), metav1.DeleteOptions{
		Preconditions: metav1.NewUIDPreconditions(string(o.GetUID())),
	}); err != nil && !errors.IsNotFound(err) {
		logger.Debugw("Error deleting object", zap.Error(err))
		return fmt.Errorf("error deleting object: %w", err)
	}

	logger.Debugw("Object has been successfully deleted", zap.Int64("results.tekton.dev/time-taken-seconds", int64(time.Since(*completionTime).Seconds())))
	return nil
}

func isDone(o results.Object) bool {
	return !o.GetStatusCondition().GetCondition(apis.ConditionSucceeded).IsUnknown()
}

// getCompletionTime returns the completion time of the object (PipelineRun or
// TaskRun) in question.
func getCompletionTime(object results.Object) (*time.Time, error) {
	var completionTime *time.Time

	switch o := object.(type) {

	case *pipelinev1beta1.PipelineRun:
		if o.Status.CompletionTime != nil {
			completionTime = &o.Status.CompletionTime.Time
		}

	case *pipelinev1beta1.TaskRun:
		if o.Status.CompletionTime != nil {
			completionTime = &o.Status.CompletionTime.Time
		}

	default:
		return nil, controller.NewPermanentError(fmt.Errorf("error getting completion time from incoming object: unrecognized type %T", o))
	}
	return completionTime, nil
}
