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
	"sync"
	"time"

	"github.com/tektoncd/results/test/performance/metrics"
)

// indexFunc processes a single dataset index, recording its own metrics.
type indexFunc func(ctx context.Context, index int, m *metrics.MetricSet)

// tickFunc runs one unit of work in a time-bounded loop, recording its own
// metrics. iter increments per call within a worker.
type tickFunc func(ctx context.Context, workerID, iter int, m *metrics.MetricSet)

// runIndexed spreads indices [start, start+count) across concurrency workers by
// stride, so a given index (and therefore a given Result) is only ever touched
// by one worker — eliminating etag contention on the store path. Each worker
// records into its own MetricSet; all are merged into the returned set.
func runIndexed(ctx context.Context, concurrency, start, count int, fn indexFunc) *metrics.MetricSet {
	if concurrency < 1 {
		concurrency = 1
	}
	total := metrics.NewMetricSet()
	sets := make([]*metrics.MetricSet, concurrency)
	for i := range sets {
		sets[i] = metrics.NewMetricSet()
	}

	total.Start()
	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			m := sets[w]
			for i := start + w; i < start+count; i += concurrency {
				if ctx.Err() != nil {
					return
				}
				fn(ctx, i, m)
			}
		}(w)
	}
	wg.Wait()
	total.Stop()

	for _, s := range sets {
		total.Merge(s)
	}
	return total
}

// runTimed runs fn repeatedly on concurrency workers until the duration elapses
// or ctx is cancelled. Used for read-heavy workloads with no natural index bound.
func runTimed(ctx context.Context, concurrency int, duration time.Duration, fn tickFunc) *metrics.MetricSet {
	if concurrency < 1 {
		concurrency = 1
	}
	runCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	total := metrics.NewMetricSet()
	sets := make([]*metrics.MetricSet, concurrency)
	for i := range sets {
		sets[i] = metrics.NewMetricSet()
	}

	total.Start()
	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			m := sets[w]
			for iter := 0; runCtx.Err() == nil; iter++ {
				fn(runCtx, w, iter, m)
			}
		}(w)
	}
	wg.Wait()
	total.Stop()

	for _, s := range sets {
		total.Merge(s)
	}
	return total
}

// observe times fn, records the latency and result under op, and returns the
// error so callers can chain (e.g. etag updates).
func observe(m *metrics.MetricSet, op string, fn func() error) error {
	start := time.Now()
	err := fn()
	m.Observe(op, time.Since(start), err)
	return err
}
