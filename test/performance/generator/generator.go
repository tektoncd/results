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

package generator

import (
	"fmt"
	"iter"
	"math/rand/v2"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	record "github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	result "github.com/tektoncd/results/pkg/api/server/v1alpha2/result"
)

// Config controls deterministic dataset generation. The same Config with the
// same Seed produces byte-identical data on every run; any change that affects
// output must bump Version so results stay comparable only within a version.
type Config struct {
	// Seed drives every random draw; fixed for reproducibility.
	Seed int64
	// Version identifies the dataset content; bump on any generator/template change.
	Version string
	// Count is the number of top-level PipelineRuns to generate.
	Count int
	// Namespaces is the fixed pool instances are spread across. Each must match
	// the Results name regex ([a-z0-9_-]{1,63}).
	Namespaces []string
	// LabelPool provides realistic label cardinality (hot values + long tail).
	LabelPool LabelPool
	// Outcomes is the terminal-state distribution (~85/10/5).
	Outcomes OutcomeRatios
	// TimeWindow is the fixed span run timestamps are spread across.
	TimeWindow TimeWindow
	// UIDs partitions the UUID space so seed and live streams never collide.
	UIDs UIDPartition
	// ChildTaskRuns clamps the per-PipelineRun child count (template may narrow it).
	ChildTaskRuns IntRange
}

// Generator materializes deterministic Instances from a Config and TemplateSet.
type Generator struct {
	cfg Config
	ts  *TemplateSet
}

// Instance is one fully-materialized top-level PipelineRun plus its child
// TaskRuns and the expected final state recorded for golden-answer computation.
type Instance struct {
	Index        int
	UID          string
	Namespace    string
	TemplateID   string
	Outcome      Outcome
	PipelineRun  *tektonv1.PipelineRun
	TaskRuns     []*tektonv1.TaskRun
	ResultName   string
	RecordName   string
	ChildRecords []string
	Labels       map[string]string
}

// New returns a Generator. If ts is nil the built-in template set is used.
func New(cfg Config, ts *TemplateSet) (*Generator, error) {
	if cfg.Count < 0 {
		return nil, fmt.Errorf("count must be non-negative, got %d", cfg.Count)
	}
	if len(cfg.Namespaces) == 0 {
		return nil, fmt.Errorf("at least one namespace is required")
	}
	if ts == nil {
		var err error
		ts, err = DefaultTemplates()
		if err != nil {
			return nil, err
		}
	}
	return &Generator{cfg: cfg, ts: ts}, nil
}

// Count returns the number of instances the generator produces.
func (g *Generator) Count() int { return g.cfg.Count }

// Config returns a copy of the generator configuration.
func (g *Generator) Config() Config { return g.cfg }

// rngFor returns an independent RNG for the given index so At(i) is reproducible
// regardless of iteration order.
func (g *Generator) rngFor(index int) *rand.Rand {
	return rand.New(rand.NewPCG(uint64(g.cfg.Seed), uint64(index))) //nolint:gosec // deterministic by design
}

// At returns the instance at the given index deterministically.
func (g *Generator) At(index int) (*Instance, error) {
	if index < 0 || index >= g.cfg.Count {
		return nil, fmt.Errorf("index %d out of range [0,%d)", index, g.cfg.Count)
	}
	rng := g.rngFor(index)

	tmpl := g.ts.pick(rng)
	ns := g.cfg.Namespaces[rng.IntN(len(g.cfg.Namespaces))]
	labels := g.cfg.LabelPool.draw(rng)
	outcome := g.cfg.Outcomes.pick(rng)
	start := g.cfg.TimeWindow.at(rng.Float64())

	childRange := clampRange(tmpl.ChildTaskRuns, g.cfg.ChildTaskRuns)
	childCount := childRange.pick(rng)

	uid := g.cfg.UIDs.uid(index)
	resultName := result.FormatName(ns, uid)
	recordName := record.FormatName(resultName, uid)

	inst := &Instance{
		Index:      index,
		UID:        uid,
		Namespace:  ns,
		TemplateID: tmpl.ID,
		Outcome:    outcome,
		ResultName: resultName,
		RecordName: recordName,
		Labels:     labels,
	}

	inst.PipelineRun = g.buildPipelineRun(tmpl, inst, start)
	inst.TaskRuns = g.buildTaskRuns(tmpl, inst, index, childCount, start)
	for _, tr := range inst.TaskRuns {
		childName := record.FormatName(resultName, string(tr.UID))
		inst.ChildRecords = append(inst.ChildRecords, childName)
	}

	return inst, nil
}

// Stream yields every instance deterministically in index order.
func (g *Generator) Stream() iter.Seq2[int, *Instance] {
	return func(yield func(int, *Instance) bool) {
		for i := 0; i < g.cfg.Count; i++ {
			inst, err := g.At(i)
			if err != nil {
				return
			}
			if !yield(i, inst) {
				return
			}
		}
	}
}

// clampRange narrows the template's child range to the global configured bounds.
func clampRange(tmpl, global IntRange) IntRange {
	out := tmpl
	if global.Max > 0 {
		if out.Max == 0 || out.Max > global.Max {
			out.Max = global.Max
		}
		if out.Min < global.Min {
			out.Min = global.Min
		}
	}
	if out.Max < out.Min {
		out.Max = out.Min
	}
	return out
}
