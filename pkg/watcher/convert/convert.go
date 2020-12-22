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
	"google.golang.org/protobuf/types/known/anypb"
)

func ToProto(in interface{}) (*anypb.Any, error) {
	var (
		m   proto.Message
		err error
	)
	switch i := in.(type) {
	case *v1beta1.TaskRun:
		m, err = ToTaskRunProto(i)
	case *v1beta1.PipelineRun:
		m, err = ToPipelineRunProto(i)
	default:
		return nil, fmt.Errorf("unsupported type %T", i)
	}
	if err != nil {
		return nil, err
	}

	return anypb.New(m)
}

// ToTaskRunProto converts a v1beta1.TaskRun object to the equivalent Results API
// proto message.
func ToTaskRunProto(tr *v1beta1.TaskRun) (*pb.TaskRun, error) {
	b, err := json.Marshal(tr)
	if err != nil {
		return nil, fmt.Errorf("error marshalling TaskRun: %v", err)
	}
	out := new(pb.TaskRun)
	m := jsonpb.Unmarshaler{
		AllowUnknownFields: true,
	}
	if err := m.Unmarshal(bytes.NewBuffer(b), out); err != nil {
		return nil, fmt.Errorf("error converting TaskRun to proto: %v", err)
	}
	return out, nil
}

// ToPipelineRunProto converts a v1beta1.PipelineRun object to the equivalent
// Results API proto message.o
func ToPipelineRunProto(pr *v1beta1.PipelineRun) (*pb.PipelineRun, error) {
	b, err := json.Marshal(pr)
	if err != nil {
		return nil, fmt.Errorf("error marshalling PipelineRun: %v", err)
	}
	out := new(pb.PipelineRun)
	m := jsonpb.Unmarshaler{
		AllowUnknownFields: true,
	}
	if err := m.Unmarshal(bytes.NewBuffer(b), out); err != nil {
		return nil, fmt.Errorf("error converting PipelineRun to proto: %v", err)
	}
	return out, nil
}
