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
	"encoding/json"
	"fmt"
	"time"

	"github.com/fatih/color"
	"github.com/jonboulle/clockwork"
	"github.com/tektoncd/cli/pkg/cli"
	tknlog "github.com/tektoncd/cli/pkg/log"
	tknopts "github.com/tektoncd/cli/pkg/options"
	"github.com/tektoncd/results/pkg/apis/v1alpha2"
	"github.com/tektoncd/results/pkg/watcher/convert"
	logwriter "github.com/tektoncd/results/pkg/watcher/log"
	"github.com/tektoncd/results/pkg/watcher/reconciler"
	"github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	"github.com/tektoncd/results/pkg/watcher/results"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
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
	log := logging.FromContext(ctx)

	if o.GetObjectKind().GroupVersionKind().Empty() {
		gvk, err := convert.InferGVK(o)
		if err != nil {
			return err
		}
		o.GetObjectKind().SetGroupVersionKind(gvk)
		log.Infof("Post-GVK Object: %v", o)
	}

	// Update record.
	result, record, err := r.resultsClient.Put(ctx, o)
	if err != nil {
		log.Errorf("error updating Record: %v", err)
		return err
	}

	var logName string

	if !o.GetObjectKind().GroupVersionKind().Empty() && o.GetObjectKind().GroupVersionKind().Kind == "TaskRun" {
		// Create a log record if the object has/supports logs
		// For now this is just TaskRuns.
		logResult, logRecord, err := r.resultsClient.PutLog(ctx, o)
		if err != nil {
			log.Errorf("error creating TaskRun log Record: %v", err)
			return err
		}
		if logRecord != nil {
			logName = logRecord.GetName()
		}
		needsStream, err := needsLogsStreamed(logRecord)
		if err != nil {
			log.Errorf("error determining if logs need to be streamed: %v", err)
			return err
		}
		if needsStream {
			log.Infof("streaming logs for TaskRun %s/%s", o.GetNamespace(), o.GetName())
			err = r.streamLogs(ctx, logResult, logRecord, o)
			if err != nil {
				log.Errorf("error streaming logs: %v", err)
				return err
			}
			log.Infof("finished streaming logs for TaskRun %s/%s", o.GetNamespace(), o.GetName())
		}
	}

	if a := o.GetAnnotations(); !r.cfg.GetDisableAnnotationUpdate() && (result.GetName() != a[annotation.Result] || record.GetName() != a[annotation.Record]) {
		// Update object with Result Annotations.
		patch, err := annotation.Add(result.GetName(), record.GetName(), logName)
		if err != nil {
			log.Errorf("error adding Result annotations: %v", err)
			return err
		}
		if err := r.objectClient.Patch(ctx, o.GetName(), types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
			log.Errorf("Patch: %v", err)
			return err
		}
	} else {
		log.Infof("skipping CRD patch - annotation patching disabled [%t] or annotations already match", r.cfg.GetDisableAnnotationUpdate())
	}

	// If the Object is complete and not yet marked for deletion, cleanup the run resource from the cluster.
	done := isDone(o)
	log.Infof("should skipping resource deletion?  - done: %t, delete enabled: %t", done, r.cfg.GetCompletedResourceGracePeriod() != 0)
	if done && r.cfg.GetCompletedResourceGracePeriod() != 0 {
		if o := o.GetOwnerReferences(); len(o) > 0 {
			log.Infof("resource is owned by another object, defering deletion to parent resource(s): %v", o)
			return nil
		}

		// We haven't hit the grace period yet - reenqueue the key for processing later.
		if s := clock.Since(record.GetUpdatedTime().AsTime()); s < r.cfg.GetCompletedResourceGracePeriod() {
			log.Infof("object is not ready for deletion - grace period: %v, time since completion: %v", r.cfg.GetCompletedResourceGracePeriod(), s)
			r.enqueue(o, r.cfg.GetCompletedResourceGracePeriod())
			return nil
		}
		log.Infof("deleting PipelineRun UID %s", o.GetUID())
		if err := r.objectClient.Delete(ctx, o.GetName(), metav1.DeleteOptions{
			Preconditions: metav1.NewUIDPreconditions(string(o.GetUID())),
		}); err != nil && !errors.IsNotFound(err) {
			log.Errorf("PipelineRun.Delete: %v", err)
			return err
		}
	} else {
		log.Infof("skipping resource deletion - done: %t, delete enabled: %t, %v", done, r.cfg.GetCompletedResourceGracePeriod() != 0, r.cfg.GetCompletedResourceGracePeriod())
	}

	return nil
}

func (r *Reconciler) streamLogs(ctx context.Context, res *pb.Result, rec *pb.Record, o metav1.Object) error {
	logClient, err := r.resultsClient.UpdateLog(ctx)
	if err != nil {
		return fmt.Errorf("failed to create PutLog client: %v", err)
	}
	writer := logwriter.NewLogWriter(logClient, rec.GetName(), logwriter.DefaultMaxLogChunkSize)

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
	// errChan receives stderr from the TaskRun containers.
	// This will be forwarded as combined output (stdout and stderr)

	// TODO: Set writer type based on the object type
	tknlog.NewWriter(tknlog.LogTypeTask, true).Write(&cli.Stream{
		Out: writer,
		Err: writer,
	}, logChan, errChan)
	return logClient.CloseSend()
}

func isDone(o results.Object) bool {
	return o.GetStatusCondition().GetCondition(apis.ConditionSucceeded).IsTrue()
}

func needsLogsStreamed(rec *pb.Record) (bool, error) {
	if rec.GetData().Type != v1alpha2.TaskRunLogRecordType {
		return false, nil
	}
	trl := &v1alpha2.TaskRunLog{}
	err := json.Unmarshal(rec.GetData().GetValue(), trl)
	if err != nil {
		return false, err
	}
	needsStream := trl.Spec.Type == v1alpha2.FileLogType
	if trl.Status.File != nil {
		needsStream = needsStream && trl.Status.File.Size <= 0
	}
	return needsStream, nil
}
