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
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
)

// isDyn returns true if the provided expression is a CEL dyn type or false
// otherwise.
func (i *interpreter) isDyn(expr *exprpb.Expr) bool {
	if theType, found := i.checkedExpr.TypeMap[expr.GetId()]; found {
		if _, ok := theType.GetTypeKind().(*exprpb.Type_Dyn); ok {
			return true
		}
	}
	return false
}

// isString returns true if the provided expression is a CEL string type or
// false otherwise.
func (i *interpreter) isString(expr *exprpb.Expr) bool {
	if theType, found := i.checkedExpr.TypeMap[expr.GetId()]; found {
		if p, ok := theType.GetTypeKind().(*exprpb.Type_Primitive); ok {
			return p.Primitive == exprpb.Type_STRING
		}
	}
	return false
}

func (i *interpreter) isRecordSummary(expr *exprpb.Expr) bool {
	if theType, found := i.checkedExpr.TypeMap[expr.GetId()]; found {
		if messageType := theType.GetMessageType(); messageType == "tekton.results.v1alpha2.RecordSummary" {
			return true
		}
	}
	return false
}

// coerceToTypeOf writes a Postgres cast directive to the current position of
// the SQL statement in the buffer, in order to cast the current SQL expression
// to the SQL type of the provided CEL expression. This feature provides
// implicit coercion to the supported expressions, by allowing users to compare
// dyn types to more specific types in a transparent manner.
//
// For instance, in the following expression:
// ```go
// data.status.completionTime > timestamp("2022/10/30T21:45:00.000Z")
// ```
// the data field is a dyn type which maps to a jsonb in the Postgres
// database. The implicit coercion casts the completionTime to a SQL timestamp
// in the returned SQL filter.
func (i *interpreter) coerceToTypeOf(expr *exprpb.Expr) error {
	if theType, found := i.checkedExpr.TypeMap[expr.GetId()]; found {
		switch theType.GetTypeKind().(type) {

		case *exprpb.Type_WellKnown:
			i.coerceWellKnownType(theType.GetWellKnown())
		}
		return nil
	}
	return ErrUnsupportedExpression
}

func (i *interpreter) coerceWellKnownType(wellKnown exprpb.Type_WellKnownType) {
	switch wellKnown {

	case exprpb.Type_TIMESTAMP:
		i.query.WriteString("::TIMESTAMP WITH TIME ZONE")

	}
}
