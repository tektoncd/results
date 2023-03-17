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

	"gorm.io/gorm/schema"
)

// translateToJSONAccessors converts the provided field path to a Postgres JSON
// property selection directive. This allows us to yield appropriate SQL
// expressions to navigate through the record.data field, for instance.
func (i *interpreter) translateToJSONAccessors(fieldPath []fmt.Stringer) {
	lastField := fieldPath[len(fieldPath)-1]
	fmt.Fprintf(&i.query, "(")
	if len(fieldPath) > 1 {
		for _, field := range fieldPath[0 : len(fieldPath)-1] {
			fmt.Fprintf(&i.query, "%s->", field)
		}
	}
	fmt.Fprintf(&i.query, ">%s", lastField)
	fmt.Fprintf(&i.query, ")")
}

// translateToRecordSummaryColumn
func (i *interpreter) translateToRecordSummaryColumn(fieldPath []fmt.Stringer) {
	namer := &schema.NamingStrategy{}
	switch f := fieldPath[1].(type) {
	case *Unquoted:
		fmt.Fprintf(&i.query, "recordsummary_%s", namer.ColumnName("", f.s))
	case *SingleQuoted:
		fmt.Fprintf(&i.query, "recordsummary_%s", namer.ColumnName("", f.s))
	}
}
