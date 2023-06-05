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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/cel-go/cel"
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
)

const (
	space = " "
)

// ErrUnsupportedExpression is a sentinel error returned when the CEL expression
// cannot be converted to a set of compatible SQL filters.
var ErrUnsupportedExpression = errors.New("unsupported CEL")

// interpreter is a statefull converter of CEL expressions to equivalent SQL
// filters in the Postgres dialect.
type interpreter struct {
	checkedExpr *exprpb.CheckedExpr
	view        *View

	query strings.Builder
}

// newInterpreter takes an abstract syntax tree and returns an Interpreter object capable
// of converting it to a set of SQL filters.
func newInterpreter(ast *cel.Ast, view *View) (*interpreter, error) {
	checkedExpr, err := cel.AstToCheckedExpr(ast)
	if err != nil {
		return nil, err
	}
	return &interpreter{
		checkedExpr: checkedExpr,
		view:        view,
	}, nil
}

// interpret attempts to convert the CEL AST into a set of valid SQL filters. It
// returns an error if the conversion cannot be done.
func (i *interpreter) interpret() (string, error) {
	if err := i.interpretExpr(i.checkedExpr.Expr); err != nil {
		return "", err
	}
	return strings.TrimSpace(i.query.String()), nil
}

func (i *interpreter) interpretExpr(expr *exprpb.Expr) error {
	id := expr.Id
	switch node := expr.ExprKind.(type) {
	case *exprpb.Expr_ConstExpr:
		return i.interpretConstExpr(id, node.ConstExpr)

	case *exprpb.Expr_IdentExpr:
		return i.interpretIdentExpr(id, node)

	case *exprpb.Expr_SelectExpr:
		return i.interpretSelectExpr(id, node)

	case *exprpb.Expr_CallExpr:
		return i.interpretCallExpr(id, expr)

	case *exprpb.Expr_ListExpr:
		return i.interpretListExpr(node)

	default:
		return i.unsupportedExprError(id, "")
	}
}

// unsupportedExprError attempts to return a descriptive error on why the
// provided CEL expression could not be converted.
func (i *interpreter) unsupportedExprError(id int64, name string) error {
	sourceInfo := i.checkedExpr.SourceInfo
	column := sourceInfo.Positions[id]
	var line int32
	for i, offset := range sourceInfo.LineOffsets {
		line = int32(i) + 1
		if offset > column {
			break
		}
	}

	if name != "" {
		name += " "
	}

	return fmt.Errorf("%w %sstatement at line %d, column %d", ErrUnsupportedExpression, name, line, column)
}

func (i *interpreter) interpretConstExpr(id int64, expr *exprpb.Constant) error {
	switch expr.ConstantKind.(type) {

	case *exprpb.Constant_NullValue:
		i.query.WriteString("NULL")

	case *exprpb.Constant_BoolValue:
		if expr.GetBoolValue() {
			i.query.WriteString("TRUE")
		} else {
			i.query.WriteString("FALSE")
		}

	case *exprpb.Constant_Int64Value:
		fmt.Fprintf(&i.query, "%d", expr.GetInt64Value())

	case *exprpb.Constant_Uint64Value:
		fmt.Fprintf(&i.query, "%d", expr.GetInt64Value())

	case *exprpb.Constant_DoubleValue:
		fmt.Fprintf(&i.query, "%f", expr.GetDoubleValue())

	case *exprpb.Constant_StringValue:
		fmt.Fprintf(&i.query, "'%s'", expr.GetStringValue())

	case *exprpb.Constant_DurationValue:
		fmt.Fprintf(&i.query, "'%d SECONDS'", expr.GetDurationValue().Seconds)

	case *exprpb.Constant_TimestampValue:
		timestamp := expr.GetTimestampValue()
		fmt.Fprintf(&i.query, "TIMESTAMP WITH TIME ZONE '%s'", timestamp.AsTime().Format(time.RFC3339))

	default:
		return i.unsupportedExprError(id, "constant")
	}
	return nil
}

func (i *interpreter) interpretIdentExpr(id int64, expr *exprpb.Expr_IdentExpr) error {
	if reference, found := i.checkedExpr.ReferenceMap[id]; found && reference.GetValue() != nil {
		return i.interpretConstExpr(id, reference.GetValue())
	}
	name := expr.IdentExpr.GetName()

	field, found := i.view.Fields[name]
	if !found {
		return i.unsupportedExprError(id, fmt.Sprintf("unexpected field %q", name))
	}

	i.query.WriteString(field.SQL)
	return nil
}

type Unquoted struct {
	s string
}

func (u *Unquoted) String() string {
	return u.s
}

type SingleQuoted struct {
	s string
}

func (s *SingleQuoted) String() string {
	return fmt.Sprintf("'%s'", s.s)
}

func (i *interpreter) getIndexKey(expr *exprpb.Expr) (fmt.Stringer, error) {
	callExprArgs := expr.GetCallExpr().GetArgs()
	lastArg := callExprArgs[len(callExprArgs)-1]
	key := lastArg.GetConstExpr()

	switch key.ConstantKind.(type) {
	case *exprpb.Constant_Int64Value:
		return &Unquoted{fmt.Sprintf("%d", key.GetInt64Value())}, nil

	case *exprpb.Constant_StringValue:
		return &SingleQuoted{key.GetStringValue()}, nil

	default:
		return nil, i.unsupportedExprError(lastArg.Id, "constant")
	}
}

func (i *interpreter) getSelectFields(expr *exprpb.Expr) ([]fmt.Stringer, *Field, error) {
	var target *exprpb.Expr
	var identField *Field
	fields := []fmt.Stringer{}
	switch node := expr.ExprKind.(type) {
	case *exprpb.Expr_SelectExpr:
		fields = append(fields, &SingleQuoted{node.SelectExpr.GetField()})
		target = node.SelectExpr.GetOperand()

	case *exprpb.Expr_CallExpr:
		if !isIndexExpr(expr) {
			// TODO: return which function is not supported
			return nil, identField, i.unsupportedExprError(expr.Id, "function")
		}
		// Sanity check, index function should always have two arguments
		if len(node.CallExpr.Args) != 2 {
			return nil, identField, ErrUnsupportedExpression
		}
		target = node.CallExpr.Args[0]
		index, err := i.getIndexKey(expr)
		if err != nil {
			return nil, identField, err
		}
		fields = append(fields, index)
	case *exprpb.Expr_IdentExpr:
		name := node.IdentExpr.GetName()
		field, found := i.view.Fields[name]
		if !found {
			return fields, identField, fmt.Errorf("unexpected field: %q", name)
		}
		fields = append(fields, &Unquoted{field.SQL})
		identField = &field
		target = nil
	default:
		return nil, identField, ErrUnsupportedExpression
	}

	if target != nil {
		newFields, field, err := i.getSelectFields(target)
		if err != nil {
			return nil, nil, err
		}
		identField = field
		fields = append(fields, newFields...)
	}

	return fields, identField, nil
}

func (i *interpreter) interpretSelectExpr(id int64, expr *exprpb.Expr_SelectExpr, additionalExprs ...*exprpb.Expr) error {
	fields, identField, err := i.getSelectFields(&exprpb.Expr{Id: id, ExprKind: expr})
	if err != nil {
		return err
	}

	reversedFields := make([]fmt.Stringer, len(fields))
	for j, k := 0, len(fields)-1; j < len(reversedFields); j, k = j+1, k-1 {
		reversedFields[j] = fields[k]
	}

	for _, node := range additionalExprs {
		switch node.ExprKind.(type) {
		case *exprpb.Expr_ConstExpr:
			reversedFields = append(reversedFields, &SingleQuoted{node.GetConstExpr().GetStringValue()})

		default:
			return ErrUnsupportedExpression
		}
	}

	if identField.ObjectType != nil {
		return i.translateIntoStruct(reversedFields)
	}

	if i.isDyn(expr.SelectExpr.GetOperand()) {
		i.translateToJSONAccessors(reversedFields)
		return nil
	}

	return fmt.Errorf("%w. %s: not recognized field", i.unsupportedExprError(id, "select"), reversedFields[0])
}

func (i *interpreter) interpretCallExpr(id int64, expr *exprpb.Expr) error {
	callExpr := expr.GetCallExpr()
	function := callExpr.GetFunction()
	if isUnaryOperator(function) {
		return i.interpretUnaryCallExpr(callExpr)
	}
	if isBinaryOperator(function) {
		return i.interpretBinaryCallExpr(expr)
	}

	if isIndexOperator(function) {
		return i.interpretIndexExpr(id, callExpr)
	}

	return i.interpretFunctionCallExpr(id, callExpr)
}

func (i *interpreter) interpretUnaryCallExpr(expr *exprpb.Expr_Call) error {
	sqlOperator := unaryOperators[expr.GetFunction()]
	i.query.WriteString(sqlOperator)
	i.query.WriteString(space)
	if err := i.interpretExpr(expr.Args[0]); err != nil {
		return err
	}
	i.query.WriteString(space)
	return nil
}

func (i *interpreter) interpretBinaryCallExpr(expr *exprpb.Expr) error {
	callExpr := expr.GetCallExpr()
	if isConcat := i.mayBeTranslatedToStringConcatExpression(callExpr); isConcat {
		return i.translateToStringConcatExpression(expr)
	}

	function := callExpr.GetFunction()
	arg1 := callExpr.Args[0]
	arg2 := callExpr.Args[1]

	if i.mayBeTranslatedToJSONPathContainsExpression(arg1, function, arg2) {
		return i.translateToJSONPathContainsExpression(arg1, arg2)
	}

	if i.mayBeTranslatedToJSONPathContainsExpression(arg2, function, arg1) {
		return i.translateToJSONPathContainsExpression(arg2, arg1)
	}

	sqlOperator := binaryOperators[function]
	if (i.isString(arg1) || i.isString(arg2)) && isAddOperator(function) {
		sqlOperator = posgresqlConcatOperator
	}

	if err := i.interpretExpr(arg1); err != nil {
		return err
	}

	// Implicit coercion
	if i.isDyn(arg1) {
		if err := i.coerceToTypeOf(arg2); err != nil {
			return err
		}
	}

	i.query.WriteString(space)
	i.query.WriteString(sqlOperator)
	i.query.WriteString(space)

	if err := i.interpretExpr(arg2); err != nil {
		return err
	}

	// Implicit coercion
	if i.isDyn(arg2) {
		if err := i.coerceToTypeOf(arg1); err != nil {
			return err
		}
	}

	return nil
}

func (i *interpreter) interpretListExpr(expr *exprpb.Expr_ListExpr) error {
	elements := expr.ListExpr.GetElements()
	i.query.WriteString("(")
	for index, elem := range elements {
		if err := i.interpretExpr(elem); err != nil {
			return err
		}
		if index < len(elements)-1 {
			i.query.WriteString(", ")
		}
	}
	i.query.WriteString(")")
	return nil
}
