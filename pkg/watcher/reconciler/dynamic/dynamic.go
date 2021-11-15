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
	"time"

	"github.com/tektoncd/results/pkg/watcher/reconciler"
	"github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	"github.com/tektoncd/results/pkg/watcher/results"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
	"knative.dev/pkg/apis"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
	"knative.dev/pkg/logging"
)

type Reconciler struct {
	client    *results.Client
	gvr       schema.GroupVersionResource
	clientset dynamic.Interface
	cfg       *reconciler.Config
	enqueue   func(interface{}, time.Duration)
}

type Object interface {
	metav1.Object
	schema.ObjectKind
}

func (r *Reconciler) Reconcile(ctx context.Context, key string) error {
	log := logging.FromContext(ctx)

	log.With(zap.String("resource", r.gvr.String()))
	log.With(zap.String("key", key))

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		log.Errorf("invalid resource key: %s", key)
		return nil
	}
	k8sclient := r.clientset.Resource(r.gvr).Namespace(namespace)

	// Lookup current Object.
	o, err := k8sclient.Get(ctx, name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		log.Warnf("resource not found: %v", err)
		return err
	}
	if err != nil {
		log.Errorf("Get: %v", err)
		return err
	}

	// Update record.
	result, record, err := r.client.Put(ctx, o)
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
		if _, err := k8sclient.Patch(ctx, o.GetName(), types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
			log.Errorf("Patch: %v", err)
			return err
		}
	} else {
		log.Infof("skipping CRD patch - annotation patching disabled [%t] or annotations already match", r.cfg.GetDisableAnnotationUpdate())
	}

	// If the Object is complete and not yet marked for deletion, cleanup the run resource from the cluster.
	done, err := isDone(o)
	if err != nil {
		log.Warnf("unable to determine done status: %v", err)
		return err
	}
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
		if err := k8sclient.Delete(ctx, o.GetName(), metav1.DeleteOptions{
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

func isDone(o *unstructured.Unstructured) (bool, error) {
	b, err := json.Marshal(o.Object["status"])
	if err != nil {
		return false, err
	}
	s := new(duckv1beta1.Status)
	if err := json.Unmarshal(b, s); err != nil {
		return false, err
	}
	return s.GetCondition(apis.ConditionSucceeded).IsTrue(), nil
}
