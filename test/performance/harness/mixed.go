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
	"sync/atomic"
	"time"

	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"github.com/tektoncd/results/test/performance/generator"
	"github.com/tektoncd/results/test/performance/metrics"
)

// mixedParams tunes the read/write split of a mixed run.
type mixedParams struct {
	Concurrency int
	Duration    time.Duration
	ReadRatio   int
	WriteRatio  int
	PageSize    int32
}

// runMixed drives writers over the live UID range and readers over the seed range
// in parallel. The ranges are disjoint by construction (see dataset.go), so reads
// return stable golden answers uncontaminated by concurrent writes, and writers
// never contend on the same Result. Writers pull distinct live indices via an
// atomic counter (each index, and thus each Result, handled once).
func runMixed(ctx context.Context, gc pb.ResultsClient, live, seed *generator.Generator, p mixedParams) *metrics.MetricSet {
	writers, readers := splitWorkers(p.Concurrency, p.ReadRatio, p.WriteRatio)

	runCtx, cancel := context.WithTimeout(ctx, p.Duration)
	defer cancel()

	total := metrics.NewMetricSet()
	total.Start()

	sets := make([]*metrics.MetricSet, 0, writers+readers)
	var mu sync.Mutex
	addSet := func(s *metrics.MetricSet) {
		mu.Lock()
		sets = append(sets, s)
		mu.Unlock()
	}

	var wg sync.WaitGroup

	// Writers: dispatch distinct live indices until exhausted or time is up.
	var nextIndex int64 = -1
	liveCount := live.Count()
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ws := metrics.NewMetricSet()
			addSet(ws)
			for runCtx.Err() == nil {
				i := int(atomic.AddInt64(&nextIndex, 1))
				if i >= liveCount {
					return
				}
				inst, err := live.At(i)
				if err != nil {
					ws.Observe("generate", 0, err)
					continue
				}
				storeInstance(runCtx, gc, inst, updateCountFor(i), ws)
			}
		}()
	}

	// Readers: loop the query mix over the seed namespaces until time is up.
	queries := defaultQueries()
	namespaces := seed.Config().Namespaces
	l := grpcLister{c: gc}
	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			rs := metrics.NewMetricSet()
			addSet(rs)
			for iter := 0; runCtx.Err() == nil; iter++ {
				q := queries[iter%len(queries)]
				ns := namespaces[(readerID+iter)%len(namespaces)]
				runQuery(runCtx, l, q, ns, p.PageSize, rs)
			}
		}(r)
	}

	wg.Wait()
	total.Stop()
	for _, s := range sets {
		total.Merge(s)
	}
	return total
}

// splitWorkers divides a worker budget between readers and writers by ratio,
// guaranteeing at least one of each when the budget allows.
func splitWorkers(total, readRatio, writeRatio int) (writers, readers int) {
	if total < 2 {
		return 1, 1
	}
	sum := readRatio + writeRatio
	if sum <= 0 {
		writers = total / 2
		return writers, total - writers
	}
	writers = total * writeRatio / sum
	if writers < 1 {
		writers = 1
	}
	if writers >= total {
		writers = total - 1
	}
	return writers, total - writers
}
