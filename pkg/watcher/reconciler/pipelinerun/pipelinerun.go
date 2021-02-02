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

	"github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	"github.com/tektoncd/results/pkg/watcher/reconciler"
	"github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	"github.com/tektoncd/results/pkg/watcher/results"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"knative.dev/pkg/logging"
)

type Reconciler struct {
	client            *results.Client
	pipelineclientset versioned.Interface
	cfg               *reconciler.Config
}

func (r *Reconciler) Reconcile(ctx context.Context, key string) error {
	log := logging.FromContext(ctx)
	log.With(zap.String("key", key))

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		log.Errorf("invalid resource key: %s", key)
		return nil
	}

	// Lookup current PipelineRun.
	pr, err := r.pipelineclientset.TektonV1beta1().PipelineRuns(namespace).Get(name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		log.Warnf("PipelineRun not found: %v", err)
		return nil
	}
	if err != nil {
		log.Errorf("PipelineRun.Get: %v", err)
		return err
	}

	// Update record.
	result, record, err := r.client.Put(ctx, pr)
	if err != nil {
		log.Errorf("error updating Record: %v", err)
		return err
	}

	if r.cfg.GetDisableAnnotationUpdate() {
		// Don't update any annotations - nothing else to do.
		return nil
	}

	if a := pr.GetAnnotations(); result.GetName() == a[annotation.Result] && record.GetName() == a[annotation.Record] {
		// Result annotations are already present in the PipelineRun.
		// Nothing else to do.
		return nil
	}

	// Update PipelineRun with Result Annotations.
	patch, err := annotation.Add(result.GetName(), record.GetName())
	if err != nil {
		log.Errorf("error adding Result annotations: %v", err)
		return err
	}
	if _, err := r.pipelineclientset.TektonV1beta1().PipelineRuns(pr.GetNamespace()).Patch(pr.Name, types.MergePatchType, patch); err != nil {
		log.Errorf("PipelineRun.Patch: %v", err)
		return err
	}
	return nil
}
