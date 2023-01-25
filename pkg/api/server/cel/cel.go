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

// Package cel provides definitions for defining the Results CEL environment.
package cel

import (
	"context"
	"log"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	ppb "github.com/tektoncd/results/proto/pipeline/v1beta1/pipeline_go_proto"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NewEnv returns the CEL environment for Results, loading in definitions for
// known types.
func NewEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Types(&pb.Result{}, &pb.Record{}, &ppb.PipelineRun{}, &ppb.TaskRun{}),
		cel.Declarations(decls.NewVar("result", decls.NewObjectType("tekton.results.v1alpha2.Result"))),
		cel.Declarations(decls.NewVar("record", decls.NewObjectType("tekton.results.v1alpha2.Record"))),
	)
}

// ParseFilter creates a CEL program based on the given filter string.
func ParseFilter(env *cel.Env, filter string) (cel.Program, error) {
	if filter == "" {
		return allowAll{}, nil
	}

	ast, issues := env.Compile(filter)
	if issues != nil && issues.Err() != nil {
		return nil, status.Errorf(codes.InvalidArgument, "error parsing filter: %v", issues.Err())
	}

	prg, err := env.Program(ast)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "error creating filter query evaluator: %v", err)
	}
	return prg, nil
}

// allowAll is a CEL program implementation that always returns true.
type allowAll struct{}

func (allowAll) ContextEval(context.Context, interface{}) (ref.Val, *cel.EvalDetails, error) {
	return types.Bool(true), nil, nil
}

func (allowAll) Eval(interface{}) (ref.Val, *cel.EvalDetails, error) {
	return types.Bool(true), nil, nil
}

// Match determines whether the given CEL filter matches the result.
func Match(prg cel.Program, data map[string]interface{}) (bool, error) {
	if prg == nil {
		return true, nil
	}
	if data == nil {
		return false, nil
	}

	out, details, err := prg.Eval(data)
	if err != nil {
		log.Printf("failed to evaluate the expression: %v", err)
		return false, status.Errorf(codes.InvalidArgument, "failed to evaluate filter: %v. Details: %+v", err, details)
	}
	b, ok := out.Value().(bool)
	if !ok {
		return false, status.Errorf(codes.InvalidArgument, "expected boolean result, got %s", out.Type().TypeName())
	}
	return b, nil
}
