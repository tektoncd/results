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
	"github.com/google/cel-go/common/operators"
)

var (
	unaryOperators = map[string]string{
		operators.Negate: "NOT",
	}

	binaryOperators = map[string]string{
		operators.LogicalAnd:    "AND",
		operators.LogicalOr:     "OR",
		operators.LogicalNot:    "NOT",
		operators.Equals:        "=",
		operators.NotEquals:     "<>",
		operators.Less:          "<",
		operators.LessEquals:    "<=",
		operators.Greater:       ">",
		operators.GreaterEquals: ">=",
		operators.Add:           "+",
		operators.Subtract:      "-",
		operators.Multiply:      "*",
		operators.Divide:        "/",
		operators.Modulo:        "%",
		operators.In:            "IN",
	}
	posgresqlConcatOperator = "||"
)

// isUnaryOperator returns true if the symbol in question is a CEL unary
// operator.
func isUnaryOperator(symbol string) bool {
	_, found := unaryOperators[symbol]
	return found
}

// isBinaryOperator returns true if the symbol in question is a CEL binary
// operator.
func isBinaryOperator(symbol string) bool {
	_, found := binaryOperators[symbol]
	return found
}

func isAddOperator(symbol string) bool {
	return symbol == operators.Add
}
