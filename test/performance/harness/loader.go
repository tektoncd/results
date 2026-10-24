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

package main

import (
	"context"
	"fmt"

	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"github.com/tektoncd/results/test/performance/generator"
	"github.com/tektoncd/results/test/performance/metrics"
	"github.com/tektoncd/results/test/performance/report"
	"google.golang.org/grpc/codes"
)

// loadSeed writes the full seed dataset through the API and returns the metrics,
// the golden definition it should match, and the actual send log. Loading reuses
// the store write path (create Result, create+finalize the top-level Record,
// create child Records) so the seed is produced through exactly the surface the
// benchmark measures.
func loadSeed(ctx context.Context, gc pb.ResultsClient, cfg *RunConfig) (*metrics.MetricSet, *generator.DatasetDefinition, *sendLog, error) {
	g, err := newGenerator(cfg, false)
	if err != nil {
		return nil, nil, nil, err
	}
	def, err := g.Definition()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("computing dataset definition: %w", err)
	}
	log := &sendLog{}
	m := runIndexed(ctx, cfg.Concurrency, 0, cfg.Count, func(ctx context.Context, index int, m *metrics.MetricSet) {
		inst, err := g.At(index)
		if err != nil {
			m.Observe("generate", 0, err)
			return
		}
		// One update finalizes the pending record to its terminal state.
		log.add(storeInstance(ctx, gc, inst, 1, m))
	})
	return m, def, log, nil
}

// summarizeSeedLoad counts each instance once if any write for its Result,
// PipelineRun, or children failed. Tolerated errors are already normalized in
// the send log.
func summarizeSeedLoad(log *sendLog) *report.SeedLoadMetrics {
	log.mu.Lock()
	defer log.mu.Unlock()
	summary := &report.SeedLoadMetrics{TotalInstances: len(log.entries)}
	for _, entry := range log.entries {
		for _, code := range entry.Codes {
			if code != codes.OK.String() {
				summary.FailedInstances++
				break
			}
		}
	}
	if summary.TotalInstances > 0 {
		summary.ErrorRatePercent = 100 * (float64(summary.FailedInstances) / float64(summary.TotalInstances))
	}
	return summary
}

// verifySeed counts the PipelineRun and TaskRun records actually present and
// compares them to the golden definition for each namespace. It returns an error
// describing any mismatch so the loader can fail loudly rather than benchmark
// against a partial or incorrectly distributed dataset.
func verifySeed(ctx context.Context, l lister, def *generator.DatasetDefinition) error {
	for ns, nc := range def.PerNamespace {
		parent := ns + "/results/-"
		gotPR, err := countRecords(ctx, l, parent, "data_type == PIPELINE_RUN")
		if err != nil {
			return fmt.Errorf("counting PipelineRun records in namespace %q: %w", ns, err)
		}
		gotTR, err := countRecords(ctx, l, parent, "data_type == TASK_RUN")
		if err != nil {
			return fmt.Errorf("counting TaskRun records in namespace %q: %w", ns, err)
		}

		if gotPR != nc.PipelineRuns || gotTR != nc.TaskRuns {
			return fmt.Errorf("row-count mismatch in namespace %q: PipelineRuns got %d want %d, TaskRuns got %d want %d", ns, gotPR, nc.PipelineRuns, gotTR, nc.TaskRuns)
		}
	}
	return nil
}

// countRecords walks every page of a ListRecords query and returns the total
// number of records matched.
func countRecords(ctx context.Context, l lister, parent, filter string) (int, error) {
	const (
		pageSize = int32(1000)
		maxPages = 100000
	)
	total := 0
	token := ""
	for page := 0; page < maxPages; page++ {
		resp, err := l.ListRecords(ctx, &pb.ListRecordsRequest{
			Parent:    parent,
			Filter:    filter,
			PageSize:  pageSize,
			PageToken: token,
		})
		if err != nil {
			return 0, err
		}
		total += len(resp.GetRecords())
		next := resp.GetNextPageToken()
		if next == "" || next == token {
			return total, nil
		}
		token = next
	}
	return total, fmt.Errorf("pagination did not terminate after %d pages", maxPages)
}
