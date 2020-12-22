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

package annotation

import (
	"encoding/json"
	"fmt"
	"strings"

	"gomodules.xyz/jsonpatch/v2"
)

const (
	Result = "results.tekton.dev/result"
	Record = "results.tekton.dev/record"
)

var (
	resultPath = path(Result)
	recordPath = path(Record)
)

func path(s string) string {
	return fmt.Sprintf("/metadata/annotations/%s", strings.ReplaceAll(s, "/", "~1"))
}

// Add creates a jsonpatch path used for adding result / record identifiers
// an object's annotations field.
func Add(result, record string) ([]byte, error) {
	patches := []jsonpatch.JsonPatchOperation{
		{
			Operation: "add",
			Path:      resultPath,
			Value:     result,
		},
		{
			Operation: "add",
			Path:      recordPath,
			Value:     record,
		},
	}
	return json.Marshal(patches)
}
