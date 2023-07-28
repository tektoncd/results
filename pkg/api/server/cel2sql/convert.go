// Copyright 2023 The Tekton Authors
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

package cel2sql

import (
	"fmt"

	"github.com/google/cel-go/cel"
)

// Convert takes a View and CEL expressions and attempt to convert them into
// Postgres SQL filters.
func Convert(view *View, filters string) (string, error) {
	env, err := view.GetEnv()
	if err != nil {
		return "", fmt.Errorf("invalid view: %w", err)
	}

	ast, issues := env.Compile(filters)
	if issues != nil && issues.Err() != nil {
		return "", fmt.Errorf("error compiling CEL filters: %w", issues.Err())
	}

	if outputType := ast.OutputType(); !outputType.IsAssignableType(cel.BoolType) {
		return "", fmt.Errorf("expected boolean expression, but got %s", outputType.String())
	}

	interpreter, err := newInterpreter(ast, view)
	if err != nil {
		return "", fmt.Errorf("error creating cel2sql interpreter: %w", err)
	}

	sql, err := interpreter.interpret()
	if err != nil {
		return "", err
	}

	return sql, nil
}
