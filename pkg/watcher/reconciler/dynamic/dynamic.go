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

	"github.com/fatih/color"
	"github.com/jonboulle/clockwork"
	"github.com/tektoncd/cli/pkg/cli"
	tknlog "github.com/tektoncd/cli/pkg/log"
	tknopts "github.com/tektoncd/cli/pkg/options"
	pipelinev1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/log"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/result"
	"github.com/tektoncd/results/pkg/logs"
	"github.com/tektoncd/results/pkg/watcher/convert"
	"github.com/tektoncd/results/pkg/watcher/reconciler"
	"github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	"github.com/tektoncd/results/pkg/watcher/results"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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
	resultsClient          *results.Client
	objectClient           ObjectClient
	cfg                    *reconciler.Config
	IsReadyForDeletionFunc IsReadyForDeletion
	AfterDeletion          AfterDeletion
}

func init() {
	// Disable colorized output from the tkn CLI.
	color.NoColor = true
}

// IsReadyForDeletion is a predicate function which indicates whether the object
// being reconciled is ready to be garbage collected. Besides the reqirements
// that are already enforced by this reconciler, callers may define more
// specific constraints by providing a function that has the below signature to
// the Reconciler instance. For instance, the controller that reconciles
// PipelineRuns can verify whether all dependent TaskRuns are up to date in the
// API server before deleting all objects in cascade.
type IsReadyForDeletion func(ctx context.Context, object results.Object) (bool, error)

// AfterDeletion is the function called after object is deleted
type AfterDeletion func(ctx context.Context, object results.Object) error

// NewDynamicReconciler creates a new dynamic Reconciler.
func NewDynamicReconciler(rc pb.ResultsClient, lc pb.LogsClient, oc ObjectClient, cfg *reconciler.Config) *Reconciler {
	return &Reconciler{
		resultsClient: results.NewClient(rc, lc),
		objectClient:  oc,
		cfg:           cfg,
		// Always true predicate.
		IsReadyForDeletionFunc: func(ctx context.Context, object results.Object) (bool, error) {
			return true, nil
		},
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
	res, rec, err := r.resultsClient.Put(ctx, o)
	timeTakenField := zap.Int64("results.tekton.dev/time-taken-ms", time.Since(startTime).Milliseconds())

	if err != nil {
		logger.Debugw("Error upserting record to API server", zap.Error(err), timeTakenField)
		return fmt.Errorf("error upserting record: %w", err)
	}

	// Update logs if enabled.
	if r.resultsClient.LogsClient != nil {
		if err := r.sendLog(ctx, o); err != nil {
			logger.Errorw("Error sending log",
				zap.String("namespace", o.GetNamespace()),
				zap.String("kind", o.GetObjectKind().GroupVersionKind().Kind),
				zap.String("name", o.GetName()),
				zap.Error(err),
			)
			return err
		}
	}

	logger = logger.With(zap.String("results.tekton.dev/result", res.Name),
		zap.String("results.tekton.dev/record", rec.Name))
	logger.Debugw("Record has been successfully upserted into API server", timeTakenField)

	recordAnnotation := annotation.Annotation{Name: annotation.Record, Value: rec.GetName()}
	resultAnnotation := annotation.Annotation{Name: annotation.Result, Value: res.GetName()}
	if err := r.addResultsAnnotations(logging.WithLogger(ctx, logger), o, recordAnnotation, resultAnnotation); err != nil {
		return err
	}

	return r.deleteUponCompletion(logging.WithLogger(ctx, logger), o)
}

// addResultsAnnotations adds Results annotations to the object in question if
// annotation patching is enabled.
func (r *Reconciler) addResultsAnnotations(ctx context.Context, o results.Object, annotations ...annotation.Annotation) error {
	logger := logging.FromContext(ctx)
	if r.cfg.GetDisableAnnotationUpdate() { //nolint:gocritic
		logger.Debug("Skipping CRD annotation patch: annotation update is disabled")
	} else if annotation.IsPatched(o, annotations...) {
		logger.Debug("Skipping CRD annotation patch: Result annotations are already set")
	} else {
		// Update object with Result Annotations.
		patch, err := annotation.Patch(o, annotations...)
		if err != nil {
			return fmt.Errorf("error adding Result annotations: %w", err)
		}
		if err := r.objectClient.Patch(ctx, o.GetName(), types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
			return fmt.Errorf("error patching object: %w", err)
		}
	}
	return nil
}

// deleteUponCompletion deletes the object in question when the following
// conditions are met:
// * The resource deletion is enabled in the config (the grace period is greater
// than 0).
// * The object is done, and it isn't owned by other object.
// * The configured grace period has elapsed since the object's completion.
// * The object satisfies all label requirements defined in the supplied config.
// * The assigned IsReadyForDeletionFunc returns true.
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
		// do not delete if the object is owned by a PipelineRun object
		// This can be removed once the PipelineRun controller is patched to stop updating the PipelineRun object
		// when child TaskRuns are deleted
		for _, or := range ownerReferences {
			if or.Kind == "PipelineRun" {
				logger.Debugw("Resource is owned by a PipelineRun, deferring deletion to parent PipelineRun", zap.Any("tekton.dev/PipelineRun", or.Name))
				return nil
			}
		}
		// do not delete if CheckOwner flag is enabled and the object has some owner references
		if r.cfg.CheckOwner {
			logger.Debugw("Resource is owned by another object, deferring deletion to parent resource(s)", zap.Any("results.tekton.dev/ownerReferences", ownerReferences))
			return nil
		}
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

	// Verify whether this object matches the provided label selectors
	if selectors := r.cfg.GetLabelSelector(); !selectors.Matches(labels.Set(o.GetLabels())) {
		logger.Debugw("Object doesn't match the required label selectors - requeuing to process later", zap.String("results.tekton.dev/label-selectors", selectors.String()))
		return controller.NewRequeueAfter(r.cfg.RequeueInterval)
	}

	if isReady, err := r.IsReadyForDeletionFunc(ctx, o); err != nil {
		return err
	} else if !isReady {
		return controller.NewRequeueAfter(r.cfg.RequeueInterval)
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
	if r.AfterDeletion != nil {
		err = r.AfterDeletion(ctx, o)
		if err != nil {
			logger.Errorw("Failed to record deletion metrics", zap.Error(err))
		}
	}
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

// sendLog streams logs to the API server
func (r *Reconciler) sendLog(ctx context.Context, o results.Object) error {
	logger := logging.FromContext(ctx)
	condition := o.GetStatusCondition().GetCondition(apis.ConditionSucceeded)
	GVK := o.GetObjectKind().GroupVersionKind()
	if !GVK.Empty() &&
		(GVK.Kind == "TaskRun" || GVK.Kind == "PipelineRun") &&
		condition != nil &&
		condition.Type == "Succeeded" &&
		!condition.IsUnknown() {

		rec, err := r.resultsClient.GetLogRecord(ctx, o)
		if err != nil {
			return err
		}
		if rec != nil {
			// we had already started logs streaming
			parent, resName, recName, err := record.ParseName(rec.GetName())
			if err != nil {
				return err
			}
			logName := log.FormatName(result.FormatName(parent, resName), recName)
			// Update log annotation if it doesn't exist
			return r.addResultsAnnotations(ctx, o, annotation.Annotation{Name: annotation.Log, Value: logName})
		}

		// Create a log record if the object has/supports logs.
		rec, err = r.resultsClient.PutLog(ctx, o)
		if err != nil {
			return err
		}

		parent, resName, recName, err := record.ParseName(rec.GetName())
		if err != nil {
			return err
		}
		logName := log.FormatName(result.FormatName(parent, resName), recName)

		var logType string
		switch o.GetObjectKind().GroupVersionKind().Kind {
		case "TaskRun":
			logType = tknlog.LogTypeTask
		case "PipelineRun":
			logType = tknlog.LogTypePipeline
		}

		if err := r.addResultsAnnotations(ctx, o, annotation.Annotation{Name: annotation.Log, Value: logName}); err != nil {
			return err
		}

		logger.Debugw("Streaming log started",
			zap.String("namespace", o.GetNamespace()),
			zap.String("kind", o.GetObjectKind().GroupVersionKind().Kind),
			zap.String("name", o.GetName()),
		)

		go func() {
			err := r.streamLogs(ctx, o, logType, logName)
			if err != nil {
				logger.Errorw("Error streaming log",
					zap.String("namespace", o.GetNamespace()),
					zap.String("kind", o.GetObjectKind().GroupVersionKind().Kind),
					zap.String("name", o.GetName()),
					zap.Error(err),
				)
			}
			logger.Debugw("Streaming log completed",
				zap.String("namespace", o.GetNamespace()),
				zap.String("kind", o.GetObjectKind().GroupVersionKind().Kind),
				zap.String("name", o.GetName()),
			)
		}()
	}

	return nil
}

func (r *Reconciler) streamLogs(ctx context.Context, o results.Object, logType, logName string) error {
	logger := logging.FromContext(ctx)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	logsClient, err := r.resultsClient.UpdateLog(ctx)
	if err != nil {
		return fmt.Errorf("failed to create UpdateLog client: %w", err)
	}

	writer := logs.NewBufferedWriter(logsClient, logName, logs.DefaultBufferSize)

	tknParams := &cli.TektonParams{}
	tknParams.SetNamespace(o.GetNamespace())
	// KLUGE: tkn reader.Read() will raise an error if a step in the TaskRun failed and there is no
	// Err writer in the Stream object. This will result in some "error" messages being written to
	// the log.

	reader, err := tknlog.NewReader(logType, &tknopts.LogOptions{
		AllSteps:        true,
		Params:          tknParams,
		PipelineRunName: o.GetName(),
		TaskrunName:     o.GetName(),
		Stream: &cli.Stream{
			Out: writer,
			Err: writer,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create tkn reader: %w", err)
	}
	logChan, errChan, err := reader.Read()
	if err != nil {
		return fmt.Errorf("error reading from tkn reader: %w", err)
	}

	errChanRepeater := make(chan error)
	go func(echan <-chan error, o metav1.Object) {
		writeErr := <-echan
		errChanRepeater <- writeErr

		_, err := writer.Flush()
		if err != nil {
			logger.Error(err)
		}
		if err = logsClient.CloseSend(); err != nil {
			logger.Error(err)
		}
	}(errChan, o)

	// errChanRepeater receives stderr from the TaskRun containers.
	// This will be forwarded as combined output (stdout and stderr)

	tknlog.NewWriter(logType, true).Write(&cli.Stream{
		Out: writer,
		Err: writer,
	}, logChan, errChanRepeater)

	return nil
}
