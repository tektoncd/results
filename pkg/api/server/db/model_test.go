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

package db

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestAnnotationsScan(t *testing.T) {
	v := make(Annotations)
	v["foo"] = "bar"

	bytes, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal(): %v", err)
	}

	var ann *Annotations
	if err := ann.Scan(bytes); err == nil {
		t.Error("annotation pointer must not be nil, expected error")
	}

	ann = &Annotations{}
	if err := ann.Scan(bytes); err != nil {
		t.Fatalf("failed to scan data from database: %v", err)
	}

	if diff := cmp.Diff(*ann, v); diff != "" {
		t.Errorf("-want, +got: %s", diff)
	}
}

func TestAnnotationsValue(t *testing.T) {
	v := make(Annotations)
	v["foo"] = "bar"

	bytes, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal(): %v", err)
	}

	annv, err := v.Value()
	if err != nil {
		t.Fatalf("Value(): %v", err)
	}

	if diff := cmp.Diff(annv, bytes); diff != "" {
		t.Errorf("-want, +got: %s", diff)
	}
}
