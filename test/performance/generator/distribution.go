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
	"math/rand/v2"
	"sort"
	"time"

	"github.com/google/uuid"
)

// hotSelectionProbability is the chance that a label value is drawn from the
// "hot" head of the pool rather than the long tail, producing realistic label
// cardinality (a few dominant values, a long tail of rare ones).
const hotSelectionProbability = 0.8

// Outcome is the terminal state of a generated run.
type Outcome string

// Terminal outcomes for generated runs.
const (
	OutcomeSucceeded Outcome = "succeeded"
	OutcomeFailed    Outcome = "failed"
	OutcomeCancelled Outcome = "cancelled"
)

// OutcomeRatios configures the distribution of terminal outcomes. The three
// fields should sum to 1.0.
type OutcomeRatios struct {
	Succeeded float64 `json:"succeeded"`
	Failed    float64 `json:"failed"`
	Cancelled float64 `json:"cancelled"`
}

// DefaultOutcomeRatios returns the ~85/10/5 split described in the story.
func DefaultOutcomeRatios() OutcomeRatios {
	return OutcomeRatios{Succeeded: 0.85, Failed: 0.10, Cancelled: 0.05}
}

// pick returns a deterministic Outcome using rng.
func (r OutcomeRatios) pick(rng *rand.Rand) Outcome {
	x := rng.Float64()
	switch {
	case x < r.Succeeded:
		return OutcomeSucceeded
	case x < r.Succeeded+r.Failed:
		return OutcomeFailed
	default:
		return OutcomeCancelled
	}
}

// LabelValues describes the value pool for a single label key. Values[:HotCount]
// are the frequently-recurring "hot" values; the remainder is the long tail.
type LabelValues struct {
	Values   []string `json:"values"`
	HotCount int      `json:"hotCount"`
}

// LabelPool maps label keys to their value distributions. The generator draws a
// value per key for each instance to create realistic label cardinality.
type LabelPool struct {
	Keys map[string]LabelValues `json:"keys"`
}

// sortedKeys returns the label keys in stable order so selection is
// reproducible regardless of Go's map iteration order.
func (p LabelPool) sortedKeys() []string {
	keys := make([]string, 0, len(p.Keys))
	for k := range p.Keys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// draw returns a deterministic {key: value} selection for one instance.
func (p LabelPool) draw(rng *rand.Rand) map[string]string {
	out := make(map[string]string, len(p.Keys))
	for _, k := range p.sortedKeys() {
		lv := p.Keys[k]
		if len(lv.Values) == 0 {
			continue
		}
		out[k] = lv.pick(rng)
	}
	return out
}

// pick selects a value biased towards the hot head of the pool.
func (lv LabelValues) pick(rng *rand.Rand) string {
	hot := lv.HotCount
	if hot <= 0 || hot > len(lv.Values) {
		hot = len(lv.Values)
	}
	tailStart := hot
	if tailStart >= len(lv.Values) || rng.Float64() < hotSelectionProbability {
		return lv.Values[rng.IntN(hot)]
	}
	return lv.Values[tailStart+rng.IntN(len(lv.Values)-tailStart)]
}

// TimeWindow is the fixed, absolute span over which run timestamps are spread.
// It must be set from configuration (never time.Now) so datasets are
// byte-reproducible.
type TimeWindow struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// at returns a deterministic instant within the window for the given fraction
// in [0,1).
func (w TimeWindow) at(fraction float64) time.Time {
	span := w.End.Sub(w.Start)
	if span <= 0 {
		return w.Start.UTC()
	}
	offset := time.Duration(fraction * float64(span))
	return w.Start.Add(offset).UTC()
}

// UIDPartition deterministically derives UUIDs from a disjoint counter range so
// the seed dataset and the live write stream never collide.
type UIDPartition struct {
	// Space is the UUIDv5 namespace the derived UIDs are hashed under.
	Space uuid.UUID `json:"space"`
	// Offset is the first counter value in this partition.
	Offset uint64 `json:"offset"`
}

// uid derives the lowercase-hex UUID for element i of the partition. The result
// always satisfies the Results name regex ([a-z0-9_-]{1,63}).
func (p UIDPartition) uid(i int) string {
	return uuid.NewSHA1(p.Space, fmt.Appendf(nil, "%d", p.Offset+uint64(i))).String() //nolint:gosec // i is a non-negative element index
}

// childUID derives the UID for child j of element i, kept disjoint from parent
// UIDs by qualifying the counter with the child index.
func (p UIDPartition) childUID(i, j int) string {
	return uuid.NewSHA1(p.Space, fmt.Appendf(nil, "%d-child-%d", p.Offset+uint64(i), j)).String() //nolint:gosec // i is a non-negative element index
}
