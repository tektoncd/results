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

// Convert takes CEL expressions and attempts to convert them into a
// parameterized Postgres SQL WHERE fragment.  It returns the SQL string
// (containing GORM-style ? placeholders), a slice of bind-parameter values,
// and any error.  Callers must pass both to GORM:
//
//	db.Where(sql, args...)
//
// This ensures that user-controlled string values are never interpolated
// directly into the SQL string, preventing SQL injection.
func Convert(env *cel.Env, filters string) (string, []any, error) {
	ast, issues := env.Compile(filters)
	if issues != nil && issues.Err() != nil {
		return "", nil, fmt.Errorf("error compiling CEL filters: %w", issues.Err())
	}

	if outputType := ast.OutputType(); !outputType.IsAssignableType(cel.BoolType) {
		return "", nil, fmt.Errorf("expected boolean expression, but got %s", outputType.String())
	}

	interpreter, err := newInterpreter(ast)
	if err != nil {
		return "", nil, fmt.Errorf("error creating cel2sql interpreter: %w", err)
	}

	return interpreter.interpret()
}
