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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatasetDefinition is the machine-readable "golden" description of a generated
// dataset: what should exist after loading it through the API. It is the
// contract the compare tool (Story 02) and compliance suite (Story 03) build on
// — golden answers such as "namespace ns-07 has exactly 412 PipelineRuns, 98
// with component=frontend" are read directly from these aggregates.
type DatasetDefinition struct {
	Version     string       `json:"version"`
	Seed        int64        `json:"seed"`
	Count       int          `json:"count"`
	ContentHash string       `json:"content_hash"`
	GeneratedAt string       `json:"generated_at"`
	UIDRange    UIDPartition `json:"uid_range"`

	Outcomes     map[string]int             `json:"outcomes"`
	PerNamespace map[string]NamespaceCounts `json:"per_namespace"`
	PerLabel     map[string]map[string]int  `json:"per_label"`
	Instances    []InstanceRecord           `json:"instances"`
}

// NamespaceCounts aggregates the records expected under a single namespace.
type NamespaceCounts struct {
	PipelineRuns int                       `json:"pipelineruns"`
	TaskRuns     int                       `json:"taskruns"`
	ByLabel      map[string]map[string]int `json:"by_label"`
}

// InstanceRecord is the expected final state of one generated PipelineRun and
// its children, keyed by UID so it joins 1:1 with the store driver's send log.
type InstanceRecord struct {
	UID          string            `json:"uid"`
	Namespace    string            `json:"namespace"`
	TemplateID   string            `json:"template_id"`
	ResultName   string            `json:"result_name"`
	RecordName   string            `json:"record_name"`
	ChildRecords []string          `json:"child_records"`
	Outcome      string            `json:"outcome"`
	Labels       map[string]string `json:"labels"`
	StartTime    string            `json:"start_time"`
	EndTime      string            `json:"end_time"`
}

// Definition materializes the full dataset and returns its golden definition,
// including a content hash over the canonical serialization of every instance.
func (g *Generator) Definition() (*DatasetDefinition, error) {
	d := &DatasetDefinition{
		Version:      g.cfg.Version,
		Seed:         g.cfg.Seed,
		Count:        g.cfg.Count,
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		UIDRange:     g.cfg.UIDs,
		Outcomes:     map[string]int{},
		PerNamespace: map[string]NamespaceCounts{},
		PerLabel:     map[string]map[string]int{},
		Instances:    make([]InstanceRecord, 0, g.cfg.Count),
	}

	hasher := sha256.New()
	for i, inst := range g.Stream() {
		if err := hashInstance(hasher, inst); err != nil {
			return nil, fmt.Errorf("hashing instance %d: %w", i, err)
		}
		d.addInstance(inst)
	}
	d.ContentHash = hex.EncodeToString(hasher.Sum(nil))
	return d, nil
}

// addInstance folds one instance into the definition aggregates.
func (d *DatasetDefinition) addInstance(inst *Instance) {
	d.Outcomes[string(inst.Outcome)]++

	nc := d.PerNamespace[inst.Namespace]
	if nc.ByLabel == nil {
		nc.ByLabel = map[string]map[string]int{}
	}
	nc.PipelineRuns++
	nc.TaskRuns += len(inst.TaskRuns)
	for k, v := range inst.Labels {
		if nc.ByLabel[k] == nil {
			nc.ByLabel[k] = map[string]int{}
		}
		nc.ByLabel[k][v]++
		if d.PerLabel[k] == nil {
			d.PerLabel[k] = map[string]int{}
		}
		d.PerLabel[k][v]++
	}
	d.PerNamespace[inst.Namespace] = nc

	d.Instances = append(d.Instances, InstanceRecord{
		UID:          inst.UID,
		Namespace:    inst.Namespace,
		TemplateID:   inst.TemplateID,
		ResultName:   inst.ResultName,
		RecordName:   inst.RecordName,
		ChildRecords: inst.ChildRecords,
		Outcome:      string(inst.Outcome),
		Labels:       inst.Labels,
		StartTime:    formatTime(inst.PipelineRun.Status.StartTime),
		EndTime:      formatTime(inst.PipelineRun.Status.CompletionTime),
	})
}

// hashInstance writes a canonical representation of an instance into h. The
// marshaled PipelineRun/TaskRun bytes are included so any change to generated
// object content changes the hash (and therefore requires a version bump).
func hashInstance(h io.Writer, inst *Instance) error {
	canonical := struct {
		UID          string   `json:"uid"`
		Namespace    string   `json:"namespace"`
		Template     string   `json:"template"`
		Outcome      string   `json:"outcome"`
		ResultName   string   `json:"result_name"`
		RecordName   string   `json:"record_name"`
		ChildRecords []string `json:"child_records"`
		Labels       []string `json:"labels"`
	}{
		UID:          inst.UID,
		Namespace:    inst.Namespace,
		Template:     inst.TemplateID,
		Outcome:      string(inst.Outcome),
		ResultName:   inst.ResultName,
		RecordName:   inst.RecordName,
		ChildRecords: inst.ChildRecords,
		Labels:       sortedLabelPairs(inst.Labels),
	}
	enc := json.NewEncoder(h)
	if err := enc.Encode(canonical); err != nil {
		return err
	}
	prBytes, err := json.Marshal(inst.PipelineRun)
	if err != nil {
		return err
	}
	if _, err := h.Write(prBytes); err != nil {
		return err
	}
	for _, tr := range inst.TaskRuns {
		trBytes, err := json.Marshal(tr)
		if err != nil {
			return err
		}
		if _, err := h.Write(trBytes); err != nil {
			return err
		}
	}
	return nil
}

// sortedLabelPairs renders labels as a stable "k=v" slice.
func sortedLabelPairs(labels map[string]string) []string {
	pairs := make([]string, 0, len(labels))
	for k, v := range labels {
		pairs = append(pairs, k+"="+v)
	}
	sort.Strings(pairs)
	return pairs
}

func formatTime(t *metav1.Time) string {
	if t == nil {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}

// WriteDefinition writes d as indented JSON.
func WriteDefinition(w io.Writer, d *DatasetDefinition) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(d)
}

// LoadDefinition reads a DatasetDefinition from JSON.
func LoadDefinition(r io.Reader) (*DatasetDefinition, error) {
	d := &DatasetDefinition{}
	if err := json.NewDecoder(r).Decode(d); err != nil {
		return nil, err
	}
	return d, nil
}
