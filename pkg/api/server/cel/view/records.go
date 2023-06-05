package view

import (
	"github.com/google/cel-go/cel"
	"github.com/tektoncd/results/pkg/api/server/cel2sql"
)

// NewRecordsView return the set of variables and constants available for CEL
// filters
func NewRecordsView() (*cel2sql.View, error) {
	view := &cel2sql.View{
		Constants: map[string]cel2sql.Constant{},
		Fields: map[string]cel2sql.Field{
			"parent": {
				CELType: cel.StringType,
				SQL:     `parent`,
			},
			"result_name": {
				CELType: cel.StringType,
				SQL:     `result_name`,
			},
			"name": {
				CELType: cel.StringType,
				SQL:     `name`,
			},
			"data_type": {
				CELType: cel.StringType,
				SQL:     `type`,
			},
			"data": {
				CELType: cel.AnyType,
				SQL:     `data`,
			},
		},
	}
	for typeName, value := range typeConstants {
		view.Constants[typeName] = value
	}
	return view, nil
}
