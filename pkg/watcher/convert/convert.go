/*
Copyright 2020 The Tekton Authors

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

// Package convert provides a method to convert v1beta1 API objects to Results
// API proto objects.
package convert

import (
	"encoding/json"
	"fmt"

	rpb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"

	"k8s.io/apimachinery/pkg/runtime"
)

func ToProto(in runtime.Object) (*rpb.Any, error) {
	if in == nil {
		return nil, nil
	}

	b, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}

	// We do not know of any formalized spec for identifying objects across API
	// versions. Standard GVK string formatting does not produce something that's
	// payload friendly (includes spaces).
	// To get around this we append API Version + Kind
	// (e.g. tekton.dev/v1beta1.TaskRun).
	v, k := in.GetObjectKind().GroupVersionKind().ToAPIVersionAndKind()
	return &rpb.Any{
		Type:  fmt.Sprintf("%s.%s", v, k),
		Value: b,
	}, nil
}
