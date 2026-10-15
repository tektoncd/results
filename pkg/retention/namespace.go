/*
Copyright 2026 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package retention

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	dberrors "github.com/tektoncd/results/pkg/api/server/db/errors"
	"github.com/tektoncd/results/pkg/apis/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// namespaceListPageSize is the page size used when listing the cluster namespaces.
const namespaceListPageSize = 500

// maxLoggedNamespaces is the maximum number of namespace names included in a
// single log message, so that a large cleanup doesn't produce an unreadable
// log entry.
const maxLoggedNamespaces = 25

// formatNamespaces renders the namespaces for a log message, keeping at most
// limit names.
func formatNamespaces(namespaces []string, limit int) string {
	if len(namespaces) <= limit {
		return strings.Join(namespaces, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(namespaces[:limit], ", "), len(namespaces)-limit)
}

// kubeNamespaceLister lists the cluster namespaces through the Kubernetes API.
type kubeNamespaceLister struct {
	client kubernetes.Interface
}

// List returns the names of all the namespaces of the cluster.
//
// ResourceVersion is deliberately left unset, so the first page is served by a
// quorum read and the continue token pins the remaining pages to that same
// snapshot. The result is therefore consistent, at the cost of the token
// expiring once the snapshot falls outside the compaction window of the API
// server, which only matters for clusters with considerably more namespaces
// than namespaceListPageSize. In that case the API server answers with a
// "continue token expired" error, the caller skips the cleanup for this run and
// the whole listing is retried on the next one.
//
// Namespaces created after the snapshot are reported as missing, but their data
// is by definition recent, so the inactivity period of the cleanup keeps it.
func (l *kubeNamespaceLister) List(ctx context.Context) ([]string, error) {
	var names []string
	opts := metav1.ListOptions{Limit: namespaceListPageSize}
	for {
		list, err := l.client.CoreV1().Namespaces().List(ctx, opts)
		if err != nil {
			return nil, err
		}
		for i := range list.Items {
			names = append(names, list.Items[i].Name)
		}
		if list.Continue == "" {
			return names, nil
		}
		opts.Continue = list.Continue
	}
}

// cleanupDeletedNamespaces removes the data of the namespaces that no longer
// exist in the cluster.
func (a *Agent) cleanupDeletedNamespaces(cfg config.NamespaceCleanup) {
	liveNamespaces, err := a.namespaces.List(a.ctx)
	if err != nil {
		a.Logger.Errorf("namespace cleanup skipped, failed to list the cluster namespaces: %s", err.Error())
		return
	}
	// An empty list is never trusted. Deleting the data of every namespace
	// because of an unexpected API server answer is not recoverable.
	if len(liveNamespaces) == 0 {
		a.Logger.Error("namespace cleanup skipped, the cluster reported no namespaces")
		return
	}

	candidates, err := a.inactiveNamespaces(cfg.InactivityPeriodDuration.Seconds())
	if err != nil {
		a.Logger.Errorf("namespace cleanup skipped, failed to list the namespaces stored in the database: %s", err.Error())
		return
	}

	keep := make(map[string]struct{}, len(liveNamespaces)+len(cfg.ExcludeNamespaces))
	for _, ns := range liveNamespaces {
		keep[ns] = struct{}{}
	}
	for _, ns := range cfg.ExcludeNamespaces {
		keep[ns] = struct{}{}
	}

	var deleted []string
	for _, ns := range candidates {
		if _, ok := keep[ns]; !ok {
			deleted = append(deleted, ns)
		}
	}
	sort.Strings(deleted)

	if len(deleted) == 0 {
		a.Logger.Info("namespace cleanup found no data belonging to deleted namespaces")
		return
	}

	if len(deleted) > cfg.MaxNamespacesPerRun {
		a.Logger.Errorf("namespace cleanup skipped and no data was deleted: %d namespaces are eligible for cleanup but maxNamespacesPerRun is %d. "+
			"Review the namespaces below and, if the cleanup is expected, raise namespaceCleanup.maxNamespacesPerRun in the %s ConfigMap, "+
			"or add the namespaces to namespaceCleanup.excludeNamespaces to keep their data. Eligible namespaces: %s",
			len(deleted), cfg.MaxNamespacesPerRun, config.GetRetentionPolicyConfigName(), formatNamespaces(deleted, maxLoggedNamespaces))
		return
	}

	if cfg.DryRun {
		a.Logger.Infof("namespace cleanup is in dry run mode, the data of the following deleted namespaces would be removed: %s",
			formatNamespaces(deleted, maxLoggedNamespaces))
		return
	}

	for _, ns := range deleted {
		rows, err := a.deleteNamespace(ns)
		if err != nil {
			a.Logger.Errorf("namespace cleanup failed to delete the data of the deleted namespace %s: %s", ns, err.Error())
			continue
		}
		a.Logger.Infof("namespace cleanup deleted %d results of the deleted namespace %s", rows, ns)
	}
}

// inactiveNamespaces returns the namespaces stored in the database whose data
// hasn't been updated for at least inactivityPeriodSeconds.
func (a *Agent) inactiveNamespaces(inactivityPeriodSeconds float64) ([]string, error) {
	query := fmt.Sprintf(`
        SELECT parent FROM results
        GROUP BY parent
        HAVING MAX(updated_time) < NOW() - INTERVAL '%f seconds'
    `, inactivityPeriodSeconds)

	var namespaces []string
	if err := dberrors.Wrap(a.db.Raw(query).Scan(&namespaces).Error); err != nil {
		return nil, err
	}
	return namespaces, nil
}

// deleteNamespace removes the results of a single namespace. The associated
// records are removed by the database through the foreign key constraint.
func (a *Agent) deleteNamespace(namespace string) (int64, error) {
	if namespace == "" {
		return 0, errors.New("the namespace mustn't be empty")
	}
	result := a.db.Exec("DELETE FROM results WHERE parent = ?", namespace)
	if err := dberrors.Wrap(result.Error); err != nil {
		return 0, err
	}
	return result.RowsAffected, nil
}
