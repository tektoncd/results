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
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/tektoncd/results/test/performance/generator"
)

// The seed and live streams draw UIDs from disjoint UUIDv5 spaces and disjoint
// counter offsets so a mixed run's writes can never collide with the seed data
// the readers query. See generator.UIDPartition.
const liveUIDOffset = 1 << 32

var (
	seedUIDSpace = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	liveUIDSpace = uuid.MustParse("22222222-2222-2222-2222-222222222222")

	// datasetWindow is a fixed absolute span; timestamps never use time.Now so
	// generated content stays byte-stable across runs and machines.
	datasetWindow = generator.TimeWindow{
		Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
	}
)

// defaultLabelPool models realistic Konflux label cardinality: a couple of hot
// values plus a long tail per key. Kept in one place so seed, live, and query
// golden answers agree.
func defaultLabelPool() generator.LabelPool {
	return generator.LabelPool{Keys: map[string]generator.LabelValues{
		"appstudio.openshift.io/component": {
			Values:   []string{"frontend", "backend", "api", "db", "cache", "worker", "ingest", "reporting", "gateway", "auth"},
			HotCount: 2,
		},
		"pipelinesascode.tekton.dev/event-type": {
			Values:   []string{"push", "pull_request", "retest", "incoming"},
			HotCount: 1,
		},
	}}
}

// buildNamespaces returns the fixed namespace pool ns-00..ns-NN.
func buildNamespaces(n int) []string {
	ns := make([]string, n)
	for i := range ns {
		ns[i] = fmt.Sprintf("ns-%02d", i)
	}
	return ns
}

// datasetConfig builds the canonical generator config. When live is true the
// UIDs come from the live partition (used by mixed writers); otherwise from the
// seed partition (used by the loader and read golden answers).
func datasetConfig(cfg *RunConfig, live bool) generator.Config {
	uids := generator.UIDPartition{Space: seedUIDSpace, Offset: 0}
	if live {
		uids = generator.UIDPartition{Space: liveUIDSpace, Offset: liveUIDOffset}
	}
	return generator.Config{
		Seed:          cfg.Seed,
		Version:       cfg.DatasetVersion,
		Count:         cfg.Count,
		Namespaces:    buildNamespaces(cfg.Namespaces),
		LabelPool:     defaultLabelPool(),
		Outcomes:      generator.DefaultOutcomeRatios(),
		TimeWindow:    datasetWindow,
		UIDs:          uids,
		ChildTaskRuns: generator.IntRange{Min: cfg.ChildMin, Max: cfg.ChildMax},
	}
}

// newGenerator builds a generator for the seed or live stream.
func newGenerator(cfg *RunConfig, live bool) (*generator.Generator, error) {
	return generator.New(datasetConfig(cfg, live), nil)
}
