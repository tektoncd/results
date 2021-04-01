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
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/golang/protobuf/jsonpb"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	pb "github.com/tektoncd/results/proto/pipeline/v1beta1/pipeline_go_proto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/runtime/protoiface"
	"google.golang.org/protobuf/types/known/anypb"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func ToProto(in interface{}) (*anypb.Any, error) {
	var (
		m   proto.Message
		err error
	)
	switch i := in.(type) {
	case *v1beta1.TaskRun:
		m, err = toTaskRunProto(i)
	case *v1beta1.PipelineRun:
		m, err = toPipelineRunProto(i)
	case *unstructured.Unstructured:
		switch {
		case matchGroupVersionKind(v1beta1.SchemeGroupVersion.WithKind("TaskRun"), i.GroupVersionKind()):
			m, err = toTaskRunProto(i)
		case matchGroupVersionKind(v1beta1.SchemeGroupVersion.WithKind("PipelineRun"), i.GroupVersionKind()):
			m, err = toPipelineRunProto(i)
		default:
			return nil, fmt.Errorf("unsupported type %s", i.GroupVersionKind().String())
		}
	default:
		return nil, fmt.Errorf("unsupported type %T", i)
	}
	if err != nil {
		return nil, err
	}

	return anypb.New(m)
}

// toTaskRunProto converts a v1beta1.TaskRun object to the equivalent Results API
// proto message.
func toTaskRunProto(i runtime.Object) (*pb.TaskRun, error) {
	pb := new(pb.TaskRun)
	err := unmarshal(i, pb)
	return pb, err
}

// ToPipelineRunProto converts a v1beta1.PipelineRun object to the equivalent
// Results API proto message.o
func toPipelineRunProto(i runtime.Object) (*pb.PipelineRun, error) {
	pb := new(pb.PipelineRun)
	err := unmarshal(i, pb)
	return pb, err
}

func unmarshal(in interface{}, out protoiface.MessageV1) error {
	b, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("error marshalling %T: %v", in, err)
	}
	m := jsonpb.Unmarshaler{
		AllowUnknownFields: true,
	}
	if err := m.Unmarshal(bytes.NewBuffer(b), out); err != nil {
		return fmt.Errorf("error converting %T to %T proto: %v", in, out, err)
	}
	return nil
}

func matchGroupVersionKind(a, b schema.GroupVersionKind) bool {
	return a.Group == b.Group && a.Version == b.Version && a.Kind == b.Kind
}
