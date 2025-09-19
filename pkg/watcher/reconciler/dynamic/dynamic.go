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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/jonboulle/clockwork"
	"github.com/tektoncd/cli/pkg/cli"
	tknlog "github.com/tektoncd/cli/pkg/log"
	tknopts "github.com/tektoncd/cli/pkg/options"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/log"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/result"
	"github.com/tektoncd/results/pkg/logs"
	"github.com/tektoncd/results/pkg/watcher/convert"
	"github.com/tektoncd/results/pkg/watcher/reconciler"
	"github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	"github.com/tektoncd/results/pkg/watcher/reconciler/client"
	"github.com/tektoncd/results/pkg/watcher/results"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
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
	// KubeClientSet allows us to talk to the k8s for core APIs
	KubeClientSet kubernetes.Interface

	resultsClient          *results.Client
	objectClient           client.ObjectClient
	cfg                    *reconciler.Config
	IsReadyForDeletionFunc IsReadyForDeletion
	AfterDeletion          AfterDeletion
	AfterStorage           AfterStorage
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

// AfterStorage is called after an object has been successfully stored
type AfterStorage func(ctx context.Context, object results.Object, storageSuccess bool) error

// NewDynamicReconciler creates a new dynamic Reconciler.
func NewDynamicReconciler(kubeClientSet kubernetes.Interface, rc pb.ResultsClient, lc pb.LogsClient, oc client.ObjectClient, cfg *reconciler.Config) *Reconciler {
	return &Reconciler{
		resultsClient: results.NewClient(rc, lc, cfg),
		KubeClientSet: kubeClientSet,
		objectClient:  oc,
		cfg:           cfg,
		// Always true predicate.
		IsReadyForDeletionFunc: func(_ context.Context, _ results.Object) (bool, error) {
			return true, nil
		},
	}
}

// Reconcile handles result/record uploading for the given Run object.
// If enabled, the object may be deleted upon successful result upload.
func (r *Reconciler) Reconcile(ctx context.Context, o results.Object) error {
	var ctxCancel context.CancelFunc
	// context with timeout does not work with the partial end to end flow that exists with unit tests;
	// this field will always be set for real
	if r.cfg != nil && r.cfg.UpdateLogTimeout != nil {
		ctx, ctxCancel = context.WithTimeout(ctx, *r.cfg.UpdateLogTimeout)
	}
	// we dont defer the dynamicCancle because golang defers follow a LIFO pattern
	// and we want to have our context analysis defer function be able to distinguish between
	// the context channel being closed because of Canceled or DeadlineExceeded
	logger := logging.FromContext(ctx)
	defer func() {
		if ctx == nil {
			return
		}
		ctxErr := ctx.Err()
		if ctxErr == nil {
			logger.Warn("Leaving dynamic Reconciler somehow but the context channel is not closed")
			return
		}
		if ctxErr == context.Canceled {
			logger.Debug("Leaving dynamic Reconciler normally with context properly canceled")
			return
		}
		if ctxErr == context.DeadlineExceeded {
			logger.Warn("Leaving dynamic Reconciler only after context timeout")
			return
		}
		logger.Warnw("Leaving dynamic Reconciler with unexpected error", zap.String("error", ctxErr.Error()))
	}()

	if o.GetObjectKind().GroupVersionKind().Empty() {
		gvk, err := convert.InferGVK(o)
		if err != nil {
			if ctxCancel != nil {
				ctxCancel()
			}
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

		if ctxCancel != nil {
			ctxCancel()
		}
		return fmt.Errorf("error upserting record: %w", err)
	}

	// Update logs if enabled.
	if r.resultsClient.LogsClient != nil {
		if r.cfg == nil || r.cfg.UpdateLogTimeout == nil {
			// single threaded for unit tests given fragility of fake k8s client
			if err = r.sendLog(ctx, o); err != nil {
				logger.Errorw("Error sending log", zap.Error(err))
			}

		} else {
			// so while performance was acceptable with development level storage mechanisms like minio, latency proved
			// intolerable for even basic amounts of log storage; moving off of the reconciler thread again, and
			// completely divesting from its context, now using the background context and a separate timer to provide
			// for timeout capability
			go func() {
				// TODO need to leverage the log status API noting log storage completion to coordinate with pruning
				backgroundCtx, cancel := context.WithCancel(context.Background())
				// need this to get grpc to clean up its threads
				defer cancel()
				timeout := 30 * time.Second
				// context with timeout does not work with the partial end to end flow that exists with unit tests;
				// this field will always be set for real
				if r.cfg != nil && r.cfg.DynamicReconcileTimeout != nil {
					// given what we have seen in stress testing, we track this timeout separately from the reconciler's timeout
					timeout = *r.cfg.DynamicReconcileTimeout
				}
				eventTicker := time.NewTicker(timeout)
				// make buffered for golang GC
				stopCh := make(chan bool, 1)
				once := sync.Once{}

				go func() {
					if err = r.sendLog(backgroundCtx, o); err != nil {
						logger.Errorw("Error sending log", zap.Error(err))
					}
					once.Do(func() { close(stopCh) })
					// TODO once we have the log status available, report the error there for retry if needed
				}()

				select {
				case <-eventTicker.C:
					once.Do(func() { close(stopCh) })
					logger.Warn("Leaving sendLogs thread only after timeout")

				case <-stopCh:
					// this is safe to call twice, as it does not need to close its buffered channel
					eventTicker.Stop()
				}
			}()

		}
	}

	// CreateEvents if enabled
	if r.cfg.StoreEvent {
		if err := r.storeEvents(ctx, o); err != nil {
			logger.Errorw("Error storing eventlist", zap.Error(err))
			if ctxCancel != nil {
				ctxCancel()
			}
			return err
		}
		logger.Debug("Successfully store eventlist")
	}
	logger = logger.With(zap.String("results.tekton.dev/result", res.Name),
		zap.String("results.tekton.dev/record", rec.Name))
	logger.Debugw("Record has been successfully upserted into API server", timeTakenField)

	recordAnnotation := annotation.Annotation{Name: annotation.Record, Value: rec.GetName()}
	resultAnnotation := annotation.Annotation{Name: annotation.Result, Value: res.GetName()}
	if err = r.addResultsAnnotations(ctx, o, recordAnnotation, resultAnnotation); err != nil {
		// no grpc calls from addResultsAnnotation
		if ctxCancel != nil {
			ctxCancel()
		}
		return err
	}

	if err = r.addChildReadyForDeletionAnnotations(ctx, o); err != nil {
		if ctxCancel != nil {
			ctxCancel()
		}
		return err
	}

	if err = r.deleteUponCompletion(ctx, o); err != nil {
		// no grpc calls from deleteUponCompletion
		if ctxCancel != nil {
			ctxCancel()
		}
		return err
	}
	if ctxCancel != nil {
		defer ctxCancel()
	}
	return r.addStoredAnnotations(ctx, o)
}

// addResultsAnnotations adds Results annotations to the object in question if
// annotation patching is enabled.
func (r *Reconciler) addResultsAnnotations(ctx context.Context, o results.Object, annotations ...annotation.Annotation) error {
	logger := logging.FromContext(ctx)
	if r.cfg.GetDisableAnnotationUpdate() { //nolint:gocritic
		logger.Debug("Skipping CRD annotation patch: annotation update is disabled")
	} else {
		err := annotation.Patch(ctx, o, r.objectClient, annotations...)
		if err != nil {
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

	case *pipelinev1.PipelineRun:
		if o.Status.CompletionTime != nil {
			completionTime = &o.Status.CompletionTime.Time
		}

	case *pipelinev1.TaskRun:
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

		logger.Debug("Streaming log started")

		err = r.streamLogs(ctx, o, logType, logName)
		if err != nil {
			logger.Errorw("Error streaming log", zap.Error(err))
			// TODO once we have the log status available, report the error there for retry if needed
		}
		logger.Info("Streaming log completed")

	}

	return nil
}

func (r *Reconciler) streamLogs(ctx context.Context, o results.Object, logType, logName string) error {
	logger := logging.FromContext(ctx)
	logsClient, err := r.resultsClient.UpdateLog(ctx)
	if err != nil {
		return fmt.Errorf("failed to create UpdateLog client: %w", err)
	}

	writer := logs.NewBufferedWriter(logsClient, logName, logs.DefaultBufferSize)

	inMemWriteBufferStdout := bytes.NewBuffer(make([]byte, 0))
	inMemWriteBufferStderr := bytes.NewBuffer(make([]byte, 0))
	tknParams := &cli.TektonParams{}
	tknParams.SetNamespace(o.GetNamespace())
	// KLUGE: tkn reader.Read() will raise an error if a step in the TaskRun failed and there is no
	// Err writer in the Stream object. This will result in some "error" messages being written to
	// the log.  That, coupled with the fact that the tkn client wrappers and oftent masks errors
	// makes it impossible to differentiate between retryable and permanent k8s errors wrt retrying
	// reconciliation in this controller

	reader, err := tknlog.NewReader(logType, &tknopts.LogOptions{
		AllSteps:        true,
		Params:          tknParams,
		PipelineRunName: o.GetName(),
		TaskrunName:     o.GetName(),
		Timestamps:      r.cfg.LogsTimestamps,
		Stream: &cli.Stream{
			Out: inMemWriteBufferStdout,
			Err: inMemWriteBufferStderr,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create tkn reader: %w", err)
	}
	logChan, errChan, err := reader.Read()
	if err != nil {
		return fmt.Errorf("error reading from tkn reader: %w", err)
	}

	tknlog.NewWriter(logType, true).Write(&cli.Stream{
		Out: inMemWriteBufferStdout,
		Err: inMemWriteBufferStderr,
	}, logChan, errChan)

	// pull the first error that occurred and return on that; reminder - per https://golang.org/ref/spec#Channel_types
	// channels act as FIFO queues
	chanErr, ok := <-errChan
	if ok && chanErr != nil {
		return fmt.Errorf("error occurred while calling tkn client write: %w", chanErr)
	}

	bufStdout := inMemWriteBufferStdout.Bytes()
	cntStdout, writeStdOutErr := writer.Write(bufStdout)
	if writeStdOutErr != nil {
		logger.Warnw("streamLogs in mem bufStdout write err", zap.String("error", writeStdOutErr.Error()))
	}
	if cntStdout != len(bufStdout) {
		logger.Warnw("streamLogs bufStdout write len inconsistent",
			zap.Int("in", len(bufStdout)),
			zap.Int("out", cntStdout),
		)

	}
	bufStderr := inMemWriteBufferStderr.Bytes()
	// we do not write these errors to the results api server

	// TODO we may need somehow discern the precise nature of the errors here and adjust how
	// we return accordingly
	if len(bufStderr) > 0 {
		errStr := string(bufStderr)
		logger.Warnw("tkn client std error output",
			zap.String("name", o.GetName()),
			zap.String("errStr", errStr))
	}

	_, flushErr := writer.Flush()
	if flushErr != nil {
		logger.Warnw("flush ret err", zap.String("error", flushErr.Error()))
		logger.Error(flushErr)
		return flushErr
	}
	// so we use CloseAndRecv vs. just CloseSent to achieve a few things:
	// 1) CloseAndRecv calls CloseSend under the covers, followed by a Recv call to obtain a LogSummary
	// 2) LogSummary appears to have some stats on the state of operations
	// 3) It also appears to be the best form of "confirmation" that the asynchronous operation of UpdateLog on the api
	// server side has reached a terminal state
	// 4) Hence, creating a child context which we cancel hopefully does not interrupt the UpdateLog call when this method exits,
	// 5) However, we need the context cancel to close out the last goroutine launched in newClientStreamWithParams that does
	// the final clean, otherwise we end up with our now familiar goroutine leak, which in the end is a memory leak

	// comparing closeErr with io.EOF does not work; and I could not find code / desc etc. constants in the grpc code that handled
	// the wrapped EOF error we expect to get from grpc when things are "OK"
	if logSummary, closeErr := logsClient.CloseAndRecv(); closeErr != nil && !strings.Contains(closeErr.Error(), "EOF") {
		logger.Warnw("CloseAndRecv ret err",
			zap.String("name", o.GetName()),
			zap.String("error", closeErr.Error()))
		if logSummary != nil {
			logger.Errorw("CloseAndRecv", zap.String("logSummary", logSummary.String()))
		}
		logger.Error(closeErr)
		return closeErr
	}

	logger.Debug("Exiting streamLogs")

	return nil
}

// storeEvents streams logs to the API server
func (r *Reconciler) storeEvents(ctx context.Context, o results.Object) error {
	logger := logging.FromContext(ctx)
	condition := o.GetStatusCondition().GetCondition(apis.ConditionSucceeded)
	GVK := o.GetObjectKind().GroupVersionKind()
	if !GVK.Empty() &&
		(GVK.Kind == "TaskRun" || GVK.Kind == "PipelineRun") &&
		condition != nil &&
		!condition.IsUnknown() {

		rec, err := r.resultsClient.GetEventListRecord(ctx, o)
		if err != nil {
			return err
		}

		if rec != nil {
			// It means we have already stored events
			eventListName := rec.GetName()
			// Update Events annotation if it doesn't exist
			return r.addResultsAnnotations(ctx, o, annotation.Annotation{Name: annotation.EventList, Value: eventListName})
		}

		events, err := r.KubeClientSet.CoreV1().Events(o.GetNamespace()).List(ctx, metav1.ListOptions{
			FieldSelector: "involvedObject.uid=" + string(o.GetUID()),
		})
		if err != nil {
			logger.Errorf("Failed to store events - retrieve", zap.String("err", err.Error()))
			return err
		}

		tr, ok := o.(*pipelinev1.TaskRun)

		if ok {
			podName := tr.Status.PodName
			podEvents, err := r.KubeClientSet.CoreV1().Events(o.GetNamespace()).List(ctx, metav1.ListOptions{
				FieldSelector: "involvedObject.name=" + podName,
			})
			if err != nil {
				logger.Errorf("Failed to fetch taskrun pod events",
					zap.String("podname", podName),
					zap.String("err", err.Error()),
				)
			}
			if podEvents != nil && len(podEvents.Items) > 0 {
				events.Items = append(events.Items, podEvents.Items...)
			}

		}

		data := filterEventList(events)
		eventList, err := json.Marshal(data)
		if err != nil {
			logger.Errorf("Failed to store events - marshal", zap.String("err", err.Error()))
			return err
		}

		rec, err = r.resultsClient.PutEventList(ctx, o, eventList)
		if err != nil {
			return err
		}

		if err := r.addResultsAnnotations(ctx, o, annotation.Annotation{Name: annotation.EventList, Value: rec.GetName()}); err != nil {
			return err
		}

	}

	return nil
}

func filterEventList(events *v1.EventList) *v1.EventList {
	if events == nil || len(events.Items) == 0 {
		return events
	}

	for i, event := range events.Items {
		// Only taking Name, Namespace and CreationTimeStamp for ObjectMeta
		events.Items[i].ObjectMeta = metav1.ObjectMeta{
			Name:              event.Name,
			Namespace:         event.Namespace,
			CreationTimestamp: event.CreationTimestamp,
		}
	}

	return events
}

// addStoredAnnotations adds stored annotations to the object in question if
// annotation patching is enabled.
func (r *Reconciler) addStoredAnnotations(ctx context.Context, o results.Object) error {
	logger := logging.FromContext(ctx)

	if r.resultsClient.LogsClient != nil {
		return nil
	}

	if r.cfg.GetDisableAnnotationUpdate() { //nolint:gocritic
		logger.Debug("Skipping CRD annotation patch: annotation update is disabled")
		return nil
	}

	stored := annotation.Annotation{Name: annotation.Stored, Value: "false"}
	GVK := o.GetObjectKind().GroupVersionKind()

	if GVK.Empty() {
		logger.Debugf("Skipping CRD annotation patch: ObjectKind is empty ObjectName: %s", o.GetName())
		return nil
	}

	// Checking if the object operation by other controllers is done
	switch GVK.Kind {
	case "TaskRun":
		taskRun, ok := o.(*pipelinev1.TaskRun)
		if !ok {
			return fmt.Errorf("failed to cast object to TaskRun")
		}
		if taskRun.IsDone() {
			stored = annotation.Annotation{Name: annotation.Stored, Value: "true"}
		}
	case "PipelineRun":
		pipelineRun, ok := o.(*pipelinev1.PipelineRun)
		if !ok {
			return fmt.Errorf("failed to cast object to PipelineRun")
		}
		if pipelineRun.IsDone() {
			stored = annotation.Annotation{Name: annotation.Stored, Value: "true"}
		}
	default:
		return nil
	}

	err := annotation.Patch(ctx, o, r.objectClient, stored)
	if err != nil {
		logger.Errorf("error patching object with stored annotation: %w ObjectName: %s", err, o.GetName())
		return fmt.Errorf("error patching object with stored annotation: %w ObjectName: %s", err, o.GetName())
	}

	// Call AfterStorage callback if this is the first time we're marking it as stored after completion
	// This ensures storage latency metrics are recorded exactly once per object when it transitions
	// from "not stored after completion" to "stored after completion"
	if stored.Value == "true" && r.AfterStorage != nil {
		logger.Debugw("Object stored after completion",
			zap.String("object", o.GetName()),
		)
		if err := r.AfterStorage(ctx, o, true); err != nil {
			logger.Warnw("Failed to call AfterStorage callback", zap.Error(err))
		}
	}

	return nil
}

// addChildReadyForDeletionAnnotations set the ChildReadyForDeletion annotation
// on objects which have an owner and are done.
func (r *Reconciler) addChildReadyForDeletionAnnotations(ctx context.Context, o results.Object) error {
	logger := logging.FromContext(ctx)
	if r.cfg.GetDisableAnnotationUpdate() { //nolint:gocritic
		logger.Debug("Skipping CRD ChildReadyForDeletion annotation patch: annotation update is disabled")
		return nil
	}

	if len(o.GetOwnerReferences()) == 0 {
		return nil
	}

	doneObj, ok := o.(interface{ IsDone() bool })
	if !ok {
		logger.Errorf("Object %s does not have IsDone() method", o.GetName())
		return fmt.Errorf("object does not have IsDone() method")
	}
	if !doneObj.IsDone() {
		logger.Debug("Skipping ChildReadyForDeletion annotation patch: object is not done yet")
		return nil
	}

	childReadyForDeletion := annotation.Annotation{Name: annotation.ChildReadyForDeletion, Value: "true"}
	err := annotation.Patch(ctx, o, r.objectClient, childReadyForDeletion)
	if err != nil {
		logger.Errorf("error patching object with ChildReadyForDeletion annotation: %w ObjectName: %s", err, o.GetName())
		return fmt.Errorf("error patching object with ChildReadyForDeletion annotation: %w ObjectName: %s", err, o.GetName())
	}

	return nil
}
