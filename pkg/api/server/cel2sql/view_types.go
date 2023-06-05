package cel2sql

import (
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	// CELTypeTimestamp allow you to set timestamp as Field's CELType.
	CELTypeTimestamp = cel.ObjectType("google.protobuf.Timestamp")
)

// Field is the configuration that allows mapping between CEL variables and
// SQL queries
type Field struct {
	// CELType is an enum to express how the field is going to be used in CEL
	CELType *cel.Type
	// SQL is a fragment that can be used to select this field in a given database
	SQL string
	// ObjectType is a special kind of type that allows CEL field selection map
	// to different columns
	ObjectType any
}

// Constant respresent a value
type Constant struct {
	StringVal *string
	Int32Val  *int32
}

// View represents a set of variables accessible by the CEL expression
type View struct {
	// Fields is a map of variable names and Field configuration
	Fields map[string]Field
	// Constants is a map of constant names and its values
	Constants map[string]Constant
}

// protoType return the protobuf type equivalent of the Constant
func (c *Constant) protoType() *exprpb.Type {
	if c.StringVal != nil {
		return decls.String
	}
	if c.Int32Val != nil {
		return decls.Int
	}
	return decls.Dyn
}

// protoConstant adapts the Constant to protobuf types
func (c *Constant) protoConstant() *exprpb.Constant {
	if c.StringVal != nil {
		return &exprpb.Constant{
			ConstantKind: &exprpb.Constant_StringValue{StringValue: *c.StringVal},
		}
	}

	if c.Int32Val != nil {
		return &exprpb.Constant{
			ConstantKind: &exprpb.Constant_Int64Value{Int64Value: int64(*c.Int32Val)},
		}
	}

	return nil
}

// GetEnv generates a new CEL environment with all variables, constants and
// types available
func (v *View) GetEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Declarations(v.celConstants()...),
		cel.Types(v.celTypes()...),
		cel.Declarations(v.celVariables()...),
	)
}

// celConstants gets all protobuf declarations of constants for a given View
func (v *View) celConstants() []*exprpb.Decl {
	constants := make([]*exprpb.Decl, 0, len(v.Constants))
	for name, value := range v.Constants {
		constants = append(constants, decls.NewConst(name, value.protoType(), value.protoConstant()))
	}
	return constants
}

// celTypes returns all custom types used in the View
func (v *View) celTypes() []any {
	types := []any{&timestamppb.Timestamp{}}
	for _, field := range v.Fields {
		if field.ObjectType != nil {
			types = append(types, field.ObjectType)
		}
	}
	return types
}

// celVariables returns all variables protobuf declarations
func (v *View) celVariables() []*exprpb.Decl {
	vars := []*exprpb.Decl{}
	for name, field := range v.Fields {
		exprType, err := cel.TypeToExprType(field.CELType)
		if err != nil {
			panic("unexpected field type in view")
		}
		vars = append(vars, decls.NewVar(name, exprType))
	}
	return vars
}
