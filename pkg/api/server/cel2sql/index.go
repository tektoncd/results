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
	"encoding/json"
	"fmt"

	"github.com/google/cel-go/common/operators"
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
)

func (i *interpreter) mayBeTranslatedToJSONPathContainsExpression(arg1 *exprpb.Expr, function string, arg2 *exprpb.Expr) bool {
	constExpr := arg2.GetConstExpr()
	if constExpr == nil {
		return false
	}
	if _, ok := constExpr.GetConstantKind().(*exprpb.Constant_StringValue); !ok {
		return false
	}
	return isIndexExpr(arg1) &&
		function == operators.Equals &&
		!i.isDyn(arg1.GetCallExpr().Args[0])
}

func isIndexExpr(expr *exprpb.Expr) bool {
	if callExpr := expr.GetCallExpr(); callExpr != nil && isIndexOperator(callExpr.GetFunction()) {
		return true
	}
	return false
}

func isIndexOperator(symbol string) bool {
	return symbol == operators.Index
}

func (i *interpreter) translateToJSONPathContainsExpression(arg1 *exprpb.Expr, arg2 *exprpb.Expr) error {
	callExprArgs := arg1.GetCallExpr().GetArgs()
	key := callExprArgs[len(callExprArgs)-1]
	for _, expr := range callExprArgs[0 : len(callExprArgs)-1] {
		if err := i.interpretExpr(expr); err != nil {
			return err
		}
	}

	jsonObj := map[string]string{
		key.GetConstExpr().GetStringValue(): arg2.GetConstExpr().GetStringValue(),
	}
	jsonBytes, err := json.Marshal(jsonObj)
	if err != nil {
		return fmt.Errorf("cel2sql: failed to marshal jsonb containment value: %w", err)
	}
	i.query.WriteString(` @> `)
	i.addParam(string(jsonBytes))
	i.query.WriteString("::jsonb")

	return nil
}

func (i *interpreter) interpretIndexExpr(id int64, expr *exprpb.Expr_Call) error {
	args := expr.GetArgs()
	if args[0].GetSelectExpr() != nil {
		return i.interpretSelectExpr(id, args[0].ExprKind.(*exprpb.Expr_SelectExpr), args[1])
	}
	if args[0].GetIdentExpr() != nil {
		if err := i.interpretExpr(args[0]); err != nil {
			return err
		}

		i.query.WriteString("->>")
		i.addParam(args[1].GetConstExpr().GetStringValue())

		return nil
	}
	return i.unsupportedExprError(args[1].Id, "index")
}
