/*
Copyright 2024 The Tekton Authors

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
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/tektoncd/results/pkg/api/server/db/errors"
	"github.com/tektoncd/results/pkg/apis/config"
)

func (a *Agent) start() {
	c := cron.New()
	_, err := c.AddFunc(a.RunAt, a.job)
	if err != nil {
		a.Logger.Fatalf("failed to add function for cronjob %s", err.Error())
	}
	a.cron = c
	a.cron.Start()
}

func (a *Agent) stop() {
	if a.cron == nil {
		return
	}
	a.cron.Stop()
}

func (a *Agent) job() {
	a.Logger.Infof("retention job started at: %s, retention policy: %+v", time.Now().String(), a.RetentionPolicy)

	caseStatement, err := buildCaseStatement(a.Policies, a.DefaultRetention)
	if err != nil {
		a.Logger.Errorf("failed to build case statement: %v", err)
		return
	}

	// First, clean up PipelineRun results.
	a.cleanupResults(caseStatement, "tekton.dev/v1.PipelineRun")

	// Second, clean up top-level TaskRun results.
	a.cleanupResults(caseStatement, "tekton.dev/v1.TaskRun")

	a.Logger.Infof("retention job finished at: %s", time.Now().String())
}

func (a *Agent) cleanupResults(caseStatement, recordType string) {
	deleteQuery := fmt.Sprintf(`
        DELETE FROM results
        WHERE id IN (
            SELECT result_id FROM (
                SELECT
                    r.result_id,
                    r.updated_time,
                    %s AS expiration_time
                FROM records r
                WHERE r.type = '%s'
            ) AS subquery
            WHERE updated_time < expiration_time
        )
    `, caseStatement, recordType)

	if err := errors.Wrap(a.db.Exec(deleteQuery).Error); err != nil {
		a.Logger.Errorf("failed to delete results for record type %s: %s", recordType, err.Error())
	}
}

func buildCaseStatement(policies []config.Policy, defaultRetention time.Duration) (string, error) {
	if len(policies) == 0 {
		return fmt.Sprintf("NOW() - INTERVAL '%f seconds'", defaultRetention.Seconds()), nil
	}
	var caseClauses []string
	for _, policy := range policies {
		whereClause, err := buildWhereClause(policy.Selector)
		if err != nil {
			return "", err
		}
		retentionDuration, err := config.ParseDuration(policy.Retention)
		if err != nil {
			return "", err
		}
		caseClauses = append(caseClauses, fmt.Sprintf("WHEN %s THEN NOW() - INTERVAL '%f seconds'", whereClause, retentionDuration.Seconds()))
	}

	defaultRetentionSeconds := defaultRetention.Seconds()
	caseClauses = append(caseClauses, fmt.Sprintf("ELSE NOW() - INTERVAL '%f seconds'", defaultRetentionSeconds))

	return fmt.Sprintf("CASE %s END", strings.Join(caseClauses, " ")), nil
}

func buildWhereClause(selector config.Selector) (string, error) {
	var conditions []string
	if len(selector.MatchNamespaces) > 0 {
		conditions = append(conditions, fmt.Sprintf("parent IN (%s)", quoteAndJoin(selector.MatchNamespaces)))
	}
	for key, values := range selector.MatchLabels {
		conditions = append(conditions, fmt.Sprintf("data->'metadata'->'labels'->>'%s' IN (%s)", key, quoteAndJoin(values)))
	}
	for key, values := range selector.MatchAnnotations {
		conditions = append(conditions, fmt.Sprintf("data->'metadata'->'annotations'->>'%s' IN (%s)", key, quoteAndJoin(values)))
	}
	if len(selector.MatchStatuses) > 0 {
		conditions = append(conditions, fmt.Sprintf("data->'status'->'conditions'->0->>'reason' IN (%s)", quoteAndJoin(selector.MatchStatuses)))
	}
	if len(conditions) == 0 {
		return "1=1", nil // No specific selectors, so match all.
	}
	return strings.Join(conditions, " AND "), nil
}

func quoteAndJoin(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = fmt.Sprintf("'%s'", item)
	}
	return strings.Join(quoted, ",")
}
