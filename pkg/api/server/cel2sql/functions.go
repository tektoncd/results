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

	"github.com/google/cel-go/common/overloads"
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
)

func (i *interpreter) interpretFunctionCallExpr(id int64, expr *exprpb.Expr_Call) error {
	function := expr.GetFunction()
	switch function {
	case overloads.Contains:
		return i.interpretContainsFunction(expr)

	case overloads.EndsWith:
		return i.translateToBinaryCall(expr, "LIKE '%' ||")

	case overloads.TimeGetDate:
		return i.translateToExtractFunctionCall(expr, "DAY", false)

	case overloads.TimeGetDayOfMonth:
		return i.translateToExtractFunctionCall(expr, "DAY", true)

	case overloads.TimeGetDayOfWeek:
		return i.translateToExtractFunctionCall(expr, "DOW", false)

	case overloads.TimeGetDayOfYear:
		return i.translateToExtractFunctionCall(expr, "DOY", true)

	case overloads.TimeGetFullYear:
		return i.translateToExtractFunctionCall(expr, "YEAR", false)

	case overloads.StartsWith:
		return i.interpretStartsWithFunction(expr)

	case overloads.Matches:
		return i.translateToBinaryCall(expr, "~")

	case overloads.TypeConvertTimestamp:
		return i.interpretTimestampFunction(expr)

	}

	return i.unsupportedExprError(id, fmt.Sprintf("`%s` function", function))
}

func (i *interpreter) interpretContainsFunction(expr *exprpb.Expr_Call) error {
	fmt.Fprintf(&i.query, "POSITION(")
	if err := i.interpretExpr(expr.Args[0]); err != nil {
		return err
	}
	fmt.Fprintf(&i.query, " IN ")
	if err := i.interpretExpr(expr.GetTarget()); err != nil {
		return err
	}
	i.query.WriteString(") <> 0")
	return nil
}

func (i *interpreter) interpretStartsWithFunction(expr *exprpb.Expr_Call) error {
	if err := i.translateToBinaryCall(expr, "LIKE"); err != nil {
		return err
	}
	i.query.WriteString(" || '%'")
	return nil
}

func (i *interpreter) translateToBinaryCall(expr *exprpb.Expr_Call, infixTerm string) error {
	if err := i.interpretExpr(expr.GetTarget()); err != nil {
		return err
	}
	fmt.Fprintf(&i.query, " %s ", infixTerm)
	if err := i.interpretExpr(expr.Args[0]); err != nil {
		return err
	}

	return nil
}

func (i *interpreter) translateToExtractFunctionCall(expr *exprpb.Expr_Call, field string, decrementReturnValue bool) error {
	if decrementReturnValue {
		i.query.WriteString("(")
	}
	fmt.Fprintf(&i.query, "EXTRACT(%s FROM ", field)
	if err := i.interpretExpr(expr.GetTarget()); err != nil {
		return err
	}
	if i.isDyn(expr.Target) {
		i.coerceWellKnownType(exprpb.Type_TIMESTAMP)
	}
	i.query.WriteString(")")
	if decrementReturnValue {
		i.query.WriteString(" - 1)")
	}
	return nil
}

func (i *interpreter) interpretTimestampFunction(expr *exprpb.Expr_Call) error {
	if err := i.interpretExpr(expr.Args[0]); err != nil {
		return err
	}
	i.query.WriteString("::TIMESTAMP WITH TIME ZONE")
	return nil
}
