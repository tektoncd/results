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
	logwriter "github.com/tektoncd/results/pkg/logwriter"
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
	enqueue       func(interface{}, time.Duration)
	ctx           context.Context
	log           *zap.SugaredLogger
}

func init() {
	// Disable colorized output from the tkn CLI.
	color.NoColor = true
}

// NewDynamicReconciler creates a new dynamic Reconciler.
func NewDynamicReconciler(rc pb.ResultsClient, oc ObjectClient, cfg *reconciler.Config, enqueue func(interface{}, time.Duration)) *Reconciler {
	return &Reconciler{
		resultsClient: &results.Client{ResultsClient: rc},
		objectClient:  oc,
		cfg:           cfg,
		enqueue:       enqueue,
	}
}

// Reconcile handles result/record uploading for the given Run object.
// If enabled, the object may be deleted upon successful result upload.
func (r *Reconciler) Reconcile(ctx context.Context, o results.Object) error {
	r.ctx = ctx
	r.log = logging.FromContext(ctx)

	if o.GetObjectKind().GroupVersionKind().Empty() {
		gvk, err := convert.InferGVK(o)
		if err != nil {
			return err
		}
		o.GetObjectKind().SetGroupVersionKind(gvk)
		r.log.Infof("Post-GVK Object: %v", o)
	}

	// Update record.
	result, record, err := r.resultsClient.Put(ctx, o)
	if err != nil {
		r.log.Errorf("error updating Record: %v", err)
		return err
	}

	if err := r.handleResultLog(o, record); err != nil {
		return err
	}

	recordAnnotation := annotation.Annotation{Name: annotation.Record, Value: record.GetName()}
	resultAnnotation := annotation.Annotation{Name: annotation.Result, Value: result.GetName()}
	if err := r.addLogAnnotation(o, recordAnnotation, resultAnnotation); err != nil {
		return err
	}

	return r.deleteObject(o, record)
}

func (r *Reconciler) handleResultLog(o results.Object, record *pb.Record) error {
	condition := o.GetStatusCondition().GetCondition(apis.ConditionSucceeded)
	if !o.GetObjectKind().GroupVersionKind().Empty() &&
		o.GetObjectKind().GroupVersionKind().Kind == "TaskRun" &&
		condition != nil &&
		condition.Type == "Succeeded" &&
		(condition.Reason == "Succeeded" || condition.Reason == "Failed") {

		isExists, err := r.resultsClient.IsLogRecordExists(r.ctx, o)
		if err != nil {
			return err
		}
		if isExists {
			// we had already started logs streaming
			return nil
		}

		// Create a log record if the object has/supports logs
		// For now this is just TaskRuns.
		_, logRec, err := r.resultsClient.PutLog(r.ctx, o)
		if err != nil {
			r.log.Errorf("error creating TaskRunLog Record: %v", err)
			return err
		}

		var logRecName string
		if record != nil {
			logRecName = logRec.GetName()
		}

		if err := r.addLogAnnotation(o, annotation.Annotation{Name: annotation.Log, Value: logRecName}); err != nil {
			return err
		}

		r.log.Infof("streaming logs for TaskRun '%s/%s'", o.GetNamespace(), o.GetName())

		go func() {
			err := r.streamLogs(o, logRecName)
			if err != nil {
				r.log.Errorf("error streaming logs: %v", err)
			}
			r.log.Infof("streaming completed '%s/%s'", o.GetNamespace(), o.GetName())
		}()
	}

	return nil
}

func (r *Reconciler) addLogAnnotation(o results.Object, annotations ...annotation.Annotation) error {
	if r.cfg.GetDisableAnnotationUpdate() {
		r.log.Infof("skipping CRD patch - annotation patching disabled [%t]", r.cfg.GetDisableAnnotationUpdate())
		return nil
	}
	var shouldBeUpdated bool
	for _, a := range annotations {
		if o.GetAnnotations()[a.Name] != a.Value {
			shouldBeUpdated = true
			break
		}
	}
	if shouldBeUpdated {
		patch, err := annotation.Add(annotations...)
		if err != nil {
			r.log.Errorf("error adding Result annotations: %v", err)
			return err
		}
		if err := r.objectClient.Patch(r.ctx, o.GetName(), types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
			r.log.Errorf("Patch: %v", err)
			return err
		}
	} else {
		r.log.Debugf("skipping CRD patch - annotations already match [%t]", r.cfg.GetDisableAnnotationUpdate())
	}
	return nil
}

func (r *Reconciler) deleteObject(o results.Object, record *pb.Record) error {
	// If the Object is complete and not yet marked for deletion, cleanup the run resource from the cluster.
	done := isDone(o)
	r.log.Infof("should skipping resource deletion?  - done: %t, delete enabled: %t", done, r.cfg.GetCompletedResourceGracePeriod() != 0)
	if done && r.cfg.GetCompletedResourceGracePeriod() != 0 {
		if o := o.GetOwnerReferences(); len(o) > 0 {
			r.log.Infof("resource is owned by another object, defering deletion to parent resource(s): %v", o)
			return nil
		}

		// We haven't hit the grace period yet - reenqueue the key for processing later.
		if s := clock.Since(record.GetUpdatedTime().AsTime()); s < r.cfg.GetCompletedResourceGracePeriod() {
			r.log.Infof("object is not ready for deletion - grace period: %v, time since completion: %v", r.cfg.GetCompletedResourceGracePeriod(), s)
			r.enqueue(o, r.cfg.GetCompletedResourceGracePeriod())
			return nil
		}
		r.log.Infof("deleting PipelineRun UID %s", o.GetUID())
		if err := r.objectClient.Delete(r.ctx, o.GetName(), metav1.DeleteOptions{
			Preconditions: metav1.NewUIDPreconditions(string(o.GetUID())),
		}); err != nil && !errors.IsNotFound(err) {
			r.log.Errorf("PipelineRun.Delete: %v", err)
			return err
		}
	} else {
		r.log.Infof("skipping resource deletion - done: %t, delete enabled: %t, %v", done, r.cfg.GetCompletedResourceGracePeriod() != 0, r.cfg.GetCompletedResourceGracePeriod())
	}

	return nil
}

func (r *Reconciler) streamLogs(o results.Object, logRecName string) error {
	logClient, err := r.resultsClient.UpdateLog(r.ctx)
	if err != nil {
		return fmt.Errorf("failed to create UpdateLog client: %v", err)
	}

	writer := logwriter.NewBufferedLogWriter(logClient, logRecName, logwriter.DefaultMaxLogChunkSize)

	tknParams := &cli.TektonParams{}
	tknParams.SetNamespace(o.GetNamespace())
	// KLUGE: tkn reader.Read() will raise an error if a step in the TaskRun failed and there is no
	// Err writer in the Stream object. This will result in some "error" messages being written to
	// the log.

	// TODO: Set TaskrunName or PipelinerunName based on object type
	reader, err := tknlog.NewReader(tknlog.LogTypeTask, &tknopts.LogOptions{
		AllSteps:    true,
		Params:      tknParams,
		TaskrunName: o.GetName(),
		Stream: &cli.Stream{
			Out: writer,
			Err: writer,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create tkn reader: %v", err)
	}
	logChan, errChan, err := reader.Read()
	if err != nil {
		return fmt.Errorf("error reading from tkn reader: %v", err)
	}

	errChanRepeater := make(chan error)
	go func(echan <-chan error, o metav1.Object) {
		writeErr := <-echan
		errChanRepeater <- writeErr

		_, err := writer.WriteRemain()
		if err != nil {
			r.log.Errorf("%v", err)
		}
		if err = logClient.CloseSend(); err != nil {
			r.log.Errorf("Failed to close stream: %v", err)
		}
	}(errChan, o)

	// errChanRepeater receives stderr from the TaskRun containers.
	// This will be forwarded as combined output (stdout and stderr)

	// TODO: Set writer type based on the object type
	tknlog.NewWriter(tknlog.LogTypeTask, true).Write(&cli.Stream{
		Out: writer,
		Err: writer,
	}, logChan, errChanRepeater)

	return nil
}

func isDone(o results.Object) bool {
	return o.GetStatusCondition().GetCondition(apis.ConditionSucceeded).IsTrue()
}
