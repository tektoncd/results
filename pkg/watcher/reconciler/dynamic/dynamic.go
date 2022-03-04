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
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/tektoncd/results/pkg/watcher/convert"
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

	if a := o.GetAnnotations(); !r.cfg.GetDisableAnnotationUpdate() && (result.GetName() != a[annotation.Result] || record.GetName() != a[annotation.Record]) {
		// Update object with Result Annotations.
		patch, err := annotation.Add(result.GetName(), record.GetName())
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

func isDone(o results.Object) bool {
	return o.GetStatusCondition().GetCondition(apis.ConditionSucceeded).IsTrue()
}
