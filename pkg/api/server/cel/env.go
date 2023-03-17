package cel

import (
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	resultspb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

const (
	typePipelineRun = "tekton.dev/v1beta1.PipelineRun"
	typeTaskRun     = "tekton.dev/v1beta1.TaskRun"
)

// NewResultsEnv creates a CEL program to build SQL filters for Result objects.
func NewResultsEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Declarations(stringConst("PIPELINE_RUN", typePipelineRun),
			stringConst("TASK_RUN", typeTaskRun),
		),
		cel.Declarations(recordSummaryStatusConsts()...),
		cel.Types(&resultspb.RecordSummary{},
			&timestamppb.Timestamp{}),
		cel.Variable("parent", cel.StringType),
		cel.Variable("uid", cel.StringType),
		cel.Variable("annotations", cel.MapType(cel.StringType, cel.StringType)),
		cel.Variable("summary",
			cel.ObjectType("tekton.results.v1alpha2.RecordSummary")),
		cel.Variable("create_time",
			cel.ObjectType("google.protobuf.Timestamp")),
		cel.Variable("update_time",
			cel.ObjectType("google.protobuf.Timestamp")),
	)
}

// NewRecordsEnv creates a CEL program to build SQL filters for Record objects.
func NewRecordsEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Declarations(stringConst("PIPELINE_RUN", typePipelineRun),
			stringConst("TASK_RUN", typeTaskRun),
		),
		cel.Variable("parent", cel.StringType),
		cel.Variable("result_name", cel.StringType),
		cel.Declarations(decls.NewVar("name", decls.String)),
		cel.Declarations(decls.NewVar("data_type", decls.String)),
		cel.Declarations(decls.NewVar("data", decls.Any)),
	)
}

// stringConst is a helper to create a CEL string constant declaration.
func stringConst(name, value string) *exprpb.Decl {
	return decls.NewConst(name,
		decls.String,
		&exprpb.Constant{ConstantKind: &exprpb.Constant_StringValue{StringValue: value}})
}

// recordSummaryStatusConsts exposes the values of the RecordSummary_Status enum
// as named constants.
func recordSummaryStatusConsts() []*exprpb.Decl {
	constants := make([]*exprpb.Decl, 0, len(resultspb.RecordSummary_Status_value))
	for name, value := range resultspb.RecordSummary_Status_value {
		constants = append(constants, decls.NewConst(name, decls.Int, &exprpb.Constant{ConstantKind: &exprpb.Constant_Int64Value{Int64Value: int64(value)}}))
	}
	return constants
}
