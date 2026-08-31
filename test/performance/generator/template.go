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

// Package generator produces deterministic, realistic PipelineRun and TaskRun
// objects for the Tekton Results performance benchmark framework. The same
// generator builds both the versioned seed dataset and the live write stream,
// so benchmark runs are comparable across milestones.
package generator

import (
	"embed"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"path"
	"sort"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"sigs.k8s.io/yaml"
)

// embeddedTemplates carries the built-in template set so the generator produces
// deterministic data without depending on files on disk. Real anonymized
// manifests are dropped into the templates/ directory following the contract
// documented in templates/README.md.
//
//go:embed all:templates
var embeddedTemplates embed.FS

// Template is a parameterized PipelineRun+TaskRun blueprint. Identity and
// distribution fields (name, namespace, UID, a subset of labels, timestamps and
// outcome) are overwritten per instance by the Generator; the rest of the
// object is kept verbatim so the JSONB size distribution stays realistic.
type Template struct {
	// ID is a stable identifier that participates in the dataset version hash.
	ID string
	// Weight is the relative probability of selecting this template.
	Weight float64
	// ChildTaskRuns bounds how many child TaskRun records are generated per
	// PipelineRun instantiated from this template.
	ChildTaskRuns IntRange

	pipelineRun *tektonv1.PipelineRun
	taskRun     *tektonv1.TaskRun
}

// IntRange is an inclusive integer range.
type IntRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// pick returns a deterministic value in [Min, Max] using rng.
func (r IntRange) pick(rng *rand.Rand) int {
	if r.Max <= r.Min {
		return r.Min
	}
	return r.Min + rng.IntN(r.Max-r.Min+1)
}

// templateMeta is the optional template.yaml sidecar describing a template.
type templateMeta struct {
	ID            string   `json:"id"`
	Weight        float64  `json:"weight"`
	ChildTaskRuns IntRange `json:"childTaskRuns"`
	Description   string   `json:"description"`
}

// TemplateSet is a loaded, weight-normalized collection of templates.
type TemplateSet struct {
	Templates   []*Template
	totalWeight float64
}

// IDs returns the template identifiers in stable (sorted) order.
func (s *TemplateSet) IDs() []string {
	ids := make([]string, 0, len(s.Templates))
	for _, t := range s.Templates {
		ids = append(ids, t.ID)
	}
	sort.Strings(ids)
	return ids
}

// pick selects a template deterministically by weight using rng.
func (s *TemplateSet) pick(rng *rand.Rand) *Template {
	if len(s.Templates) == 1 {
		return s.Templates[0]
	}
	target := rng.Float64() * s.totalWeight
	var cum float64
	for _, t := range s.Templates {
		cum += t.Weight
		if target < cum {
			return t
		}
	}
	return s.Templates[len(s.Templates)-1]
}

// DefaultTemplates loads the template set bundled into the binary.
func DefaultTemplates() (*TemplateSet, error) {
	return LoadTemplates(embeddedTemplates, "templates")
}

// LoadTemplates reads every template directory under root in fsys. Each template
// lives in its own subdirectory containing at minimum a pipelinerun.yaml; an
// optional taskrun.yaml provides the child TaskRun skeleton and an optional
// template.yaml carries metadata (id, weight, child count range).
func LoadTemplates(fsys fs.FS, root string) (*TemplateSet, error) {
	entries, err := fs.ReadDir(fsys, root)
	if err != nil {
		return nil, fmt.Errorf("reading template root %q: %w", root, err)
	}

	set := &TemplateSet{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := path.Join(root, e.Name())
		t, err := loadTemplate(fsys, dir, e.Name())
		if err != nil {
			return nil, err
		}
		if t == nil {
			continue
		}
		set.Templates = append(set.Templates, t)
		set.totalWeight += t.Weight
	}

	if len(set.Templates) == 0 {
		return nil, fmt.Errorf("no templates found under %q (each template needs its own directory with a pipelinerun.yaml)", root)
	}
	// Stable ordering keeps template selection reproducible across runs.
	sort.Slice(set.Templates, func(i, j int) bool { return set.Templates[i].ID < set.Templates[j].ID })
	return set, nil
}

// loadTemplate loads a single template directory. It returns (nil, nil) when the
// directory does not contain a pipelinerun.yaml so unrelated directories are
// skipped silently.
func loadTemplate(fsys fs.FS, dir, name string) (*Template, error) {
	prBytes, err := fs.ReadFile(fsys, path.Join(dir, "pipelinerun.yaml"))
	if err != nil {
		return nil, nil //nolint:nilerr // directory without a PipelineRun is not a template
	}

	pr := &tektonv1.PipelineRun{}
	if err := yaml.UnmarshalStrict(prBytes, pr); err != nil {
		return nil, fmt.Errorf("parsing %s/pipelinerun.yaml: %w", dir, err)
	}

	t := &Template{
		ID:            name,
		Weight:        1,
		ChildTaskRuns: IntRange{Min: 2, Max: 15},
		pipelineRun:   pr,
	}

	if trBytes, err := fs.ReadFile(fsys, path.Join(dir, "taskrun.yaml")); err == nil {
		tr := &tektonv1.TaskRun{}
		if err := yaml.UnmarshalStrict(trBytes, tr); err != nil {
			return nil, fmt.Errorf("parsing %s/taskrun.yaml: %w", dir, err)
		}
		t.taskRun = tr
	}

	if metaBytes, err := fs.ReadFile(fsys, path.Join(dir, "template.yaml")); err == nil {
		meta := templateMeta{}
		if err := yaml.UnmarshalStrict(metaBytes, &meta); err != nil {
			return nil, fmt.Errorf("parsing %s/template.yaml: %w", dir, err)
		}
		if meta.ID != "" {
			t.ID = meta.ID
		}
		if meta.Weight > 0 {
			t.Weight = meta.Weight
		}
		if meta.ChildTaskRuns.Max > 0 {
			t.ChildTaskRuns = meta.ChildTaskRuns
		}
	}

	return t, nil
}
