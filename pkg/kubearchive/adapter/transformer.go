// Copyright 2020 The Tekton Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package adapter

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tektoncd/results/pkg/api/server/db"
)

// ResourceTransformer transforms Tekton Results Record format to Kubernetes resource JSON
type ResourceTransformer struct{}

// RecordToK8sResource converts a Tekton Results Record to a Kubernetes resource JSON string
func (t *ResourceTransformer) RecordToK8sResource(record *db.Record) (string, error) {
	// Unmarshal the Record.Data which contains the Tekton resource
	var tektonResource map[string]interface{}
	if err := json.Unmarshal(record.Data, &tektonResource); err != nil {
		return "", fmt.Errorf("failed to unmarshal record data: %w", err)
	}

	// Parse Type field to extract apiVersion and kind
	// Format: "{apiGroup}/{version}.{Kind}" e.g., "tekton.dev/v1.TaskRun"
	apiVersion, kind, err := parseType(record.Type)
	if err != nil {
		return "", fmt.Errorf("failed to parse type %q: %w", record.Type, err)
	}

	// Build full Kubernetes resource structure
	k8sResource := map[string]interface{}{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata": map[string]interface{}{
			"name":              record.Name,
			"namespace":         record.Parent,
			"uid":               record.ID,
			"creationTimestamp": record.CreatedTime.UTC().Format(time.RFC3339),
		},
	}

	// Merge existing data from the tektonResource (spec, status, etc.)
	// Skip apiVersion and kind as we've already set them
	for key, value := range tektonResource {
		if key != "apiVersion" && key != "kind" {
			k8sResource[key] = value
		}
	}

	// If the original resource had metadata fields we haven't set, merge them
	if metadata, ok := tektonResource["metadata"].(map[string]interface{}); ok {
		k8sMeta := k8sResource["metadata"].(map[string]interface{})
		for key, value := range metadata {
			// Don't overwrite our core fields
			if key != "name" && key != "namespace" && key != "uid" && key != "creationTimestamp" {
				k8sMeta[key] = value
			}
		}
	}

	// Marshal to JSON string
	jsonBytes, err := json.Marshal(k8sResource)
	if err != nil {
		return "", fmt.Errorf("failed to marshal K8s resource: %w", err)
	}

	return string(jsonBytes), nil
}

// parseType parses the Type field from Tekton Results Record
// Input format: "{apiGroup}/{version}.{Kind}" e.g., "tekton.dev/v1.TaskRun"
// Returns: apiVersion ("{apiGroup}/{version}"), kind ("{Kind}"), error
func parseType(typeStr string) (apiVersion, kind string, err error) {
	if typeStr == "" {
		return "", "", fmt.Errorf("type string is empty")
	}

	// Find the last dot which separates apiVersion from Kind
	lastDot := strings.LastIndex(typeStr, ".")
	if lastDot == -1 {
		return "", "", fmt.Errorf("invalid type format: no dot separator found")
	}

	apiVersion = typeStr[:lastDot]
	kind = typeStr[lastDot+1:]

	if apiVersion == "" || kind == "" {
		return "", "", fmt.Errorf("invalid type format: empty apiVersion or kind")
	}

	return apiVersion, kind, nil
}
