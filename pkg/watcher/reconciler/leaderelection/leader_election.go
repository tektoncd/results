// Copyright 2022 The Tekton Authors
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

// Package leaderelection provides a few utilities to help us to enable leader
// election support in the Watcher controllers.
package leaderelection

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/reconciler"
)

// Lister is a generic signature of Lister.List functions, by allowing us to
// support various listers in the NewLeaderAwareFuncs function below.
type Lister[O metav1.Object] func(labels.Selector) ([]O, error)

// NewLeaderAwareFuncs returns a new reconciler.LeaderAwareFuncs object to be
// used in our controllers.
func NewLeaderAwareFuncs[O metav1.Object](lister Lister[O]) reconciler.LeaderAwareFuncs {
	return reconciler.LeaderAwareFuncs{
		PromoteFunc: func(bucket reconciler.Bucket, enqueue func(reconciler.Bucket, types.NamespacedName)) error {
			objects, err := lister(labels.Everything())
			if err != nil {
				return err
			}
			for _, object := range objects {
				enqueue(bucket, types.NamespacedName{
					Namespace: object.GetNamespace(),
					Name:      object.GetName(),
				})
			}
			return nil
		},
	}
}
