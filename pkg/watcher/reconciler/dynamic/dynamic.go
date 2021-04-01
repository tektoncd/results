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

	"github.com/tektoncd/results/pkg/watcher/reconciler"
	"github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	"github.com/tektoncd/results/pkg/watcher/results"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
	"knative.dev/pkg/logging"
)

type Reconciler struct {
	client    *results.Client
	gvr       schema.GroupVersionResource
	clientset dynamic.Interface
	cfg       *reconciler.Config
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

	// Lookup current Object.
	o, err := r.clientset.Resource(r.gvr).Namespace(namespace).Get(name, metav1.GetOptions{})
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

	if r.cfg.GetDisableAnnotationUpdate() {
		// Don't update any annotations - nothing else to do.
		return nil
	}

	if a := o.GetAnnotations(); result.GetName() == a[annotation.Result] && record.GetName() == a[annotation.Record] {
		// Result annotations are already present in the Object.
		// Nothing else to do.
		return nil
	}

	// Update Object with Result Annotations.
	patch, err := annotation.Add(result.GetName(), record.GetName())
	if err != nil {
		log.Errorf("error adding Result annotations: %v", err)
		return err
	}
	if _, err := r.clientset.Resource(r.gvr).Namespace(namespace).Patch(o.GetName(), types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
		log.Errorf("Patch: %v", err)
		return err
	}
	return nil
}
