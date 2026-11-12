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

package convert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// protectedFields are needed to identify a stored object and cannot be
// stripped, either directly or by removing one of their parents.
var protectedFields = []string{
	"apiVersion",
	"kind",
	"metadata.name",
	"metadata.namespace",
	"metadata.uid",
}

// ParseFieldPaths parses a comma-separated list of dot-separated field paths,
// such as "metadata.managedFields,status.provenance". Surrounding spaces and
// empty entries are ignored. Paths that would remove a field needed to
// identify the object are rejected.
func ParseFieldPaths(raw string) ([][]string, error) {
	var paths [][]string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		segments := strings.Split(p, ".")
		for _, s := range segments {
			if s == "" {
				return nil, fmt.Errorf("invalid field path %q: empty segment", p)
			}
		}
		for _, protected := range protectedFields {
			if protected == p || strings.HasPrefix(protected, p+".") {
				return nil, fmt.Errorf("field path %q cannot be stripped: %s is needed to identify the object", p, protected)
			}
		}
		paths = append(paths, segments)
	}
	return paths, nil
}

// StripFields removes the given field paths from a JSON object and returns the
// re-encoded object. Paths that do not exist are ignored. Paths are only
// followed through JSON objects, not through arrays.
func StripFields(data []byte, paths [][]string) ([]byte, error) {
	if len(paths) == 0 {
		return data, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	// Keep numbers as written, so large integers do not lose precision.
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, fmt.Errorf("decoding object to strip fields: %w", err)
	}
	for _, path := range paths {
		deletePath(obj, path)
	}
	return json.Marshal(obj)
}

func deletePath(obj map[string]any, path []string) {
	for _, key := range path[:len(path)-1] {
		next, ok := obj[key].(map[string]any)
		if !ok {
			return
		}
		obj = next
	}
	delete(obj, path[len(path)-1])
}
