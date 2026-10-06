// Copyright 2026 The Tekton Authors
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

package namespace

import (
	"context"
	"fmt"

	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/reconciler"
)

const maxPageSize = 10000

// Reconciler cleans up Results API data when a namespace is deleted.
type Reconciler struct {
	reconciler.LeaderAwareFuncs
	resultsClient   pb.ResultsClient
	namespaceLister corev1listers.NamespaceLister
}

// Reconcile handles namespace events, deleting associated Results when a namespace is deleted.
func (r *Reconciler) Reconcile(ctx context.Context, key string) error {
	logger := logging.FromContext(ctx)
	namespaceName := key

	ns, err := r.namespaceLister.Get(namespaceName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("error getting namespace %s: %w", namespaceName, err)
		}
	}

	if ns != nil && ns.DeletionTimestamp == nil {
		return nil
	}

	logger.Infof("Namespace %s deleted, cleaning up Results", namespaceName)
	return r.deleteResultsForNamespace(ctx, namespaceName)
}

func (r *Reconciler) deleteResultsForNamespace(ctx context.Context, namespace string) error {
	logger := logging.FromContext(ctx)

	var totalDeleted int
	pageToken := ""

	for {
		resp, err := r.resultsClient.ListResults(ctx, &pb.ListResultsRequest{
			Parent:    namespace,
			PageSize:  maxPageSize,
			PageToken: pageToken,
		})
		if err != nil {
			return fmt.Errorf("error listing results for namespace %s: %w", namespace, err)
		}

		for _, result := range resp.GetResults() {
			_, err := r.resultsClient.DeleteResult(ctx, &pb.DeleteResultRequest{
				Name: result.GetName(),
			})
			if err != nil {
				logger.Warnf("Failed to delete result %s: %v", result.GetName(), err)
				continue
			}
			totalDeleted++
		}

		pageToken = resp.GetNextPageToken()
		if pageToken == "" {
			break
		}
	}

	logger.Infof("Cleaned up %d results for deleted namespace %s", totalDeleted, namespace)
	return nil
}
