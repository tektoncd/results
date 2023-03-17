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

	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
)

// mayBeTranslatedToStringConcatExpression returns a boolean whether the
// expression is a string concatenation. If it is, returns both arguments.
func (i *interpreter) mayBeTranslatedToStringConcatExpression(expr *exprpb.Expr_Call) bool {
	if function := expr.GetFunction(); !isAddOperator(function) {
		return false
	}
	arg1 := expr.Args[0]
	arg2 := expr.Args[1]
	return i.isString(arg1) || i.isString(arg2)
}

func (i interpreter) allStringConcatArgs(expr *exprpb.Expr) []*exprpb.Expr {
	args := []*exprpb.Expr{}
	switch node := expr.ExprKind.(type) {
	case *exprpb.Expr_CallExpr:
		if isAddOperator(node.CallExpr.Function) {
			arg1 := node.CallExpr.Args[0]
			arg2 := node.CallExpr.Args[1]
			args = append(args, i.allStringConcatArgs(arg1)...)
			args = append(args, i.allStringConcatArgs(arg2)...)
		}
	default:
		args = append(args, expr)
	}
	return args
}

func (i *interpreter) translateToStringConcatExpression(expr *exprpb.Expr) error {
	args := i.allStringConcatArgs(expr)
	fmt.Fprintf(&i.query, "CONCAT(")
	for j, arg := range args {
		err := i.interpretExpr(arg)
		if err != nil {
			return err
		}
		if j != len(args)-1 {
			fmt.Fprintf(&i.query, ", ")
		}
	}
	fmt.Fprintf(&i.query, ")")
	return nil
}
