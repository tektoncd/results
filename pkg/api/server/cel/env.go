package cel

import (
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	resultspb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	typePipelineRun = "tekton.dev/v1.PipelineRun"
	typeTaskRun     = "tekton.dev/v1.TaskRun"
)

// NewResultsEnv creates a CEL program to build SQL filters for Result objects.
func NewResultsEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Constant("PIPELINE_RUN", cel.StringType, types.String(typePipelineRun)),
		cel.Constant("TASK_RUN", cel.StringType, types.String(typeTaskRun)),
		cel.Constant("UNKNOWN", cel.IntType, types.Int(resultspb.RecordSummary_UNKNOWN)),
		cel.Constant("SUCCESS", cel.IntType, types.Int(resultspb.RecordSummary_SUCCESS)),
		cel.Constant("FAILURE", cel.IntType, types.Int(resultspb.RecordSummary_FAILURE)),
		cel.Constant("TIMEOUT", cel.IntType, types.Int(resultspb.RecordSummary_TIMEOUT)),
		cel.Constant("CANCELLED", cel.IntType, types.Int(resultspb.RecordSummary_CANCELLED)),
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
		cel.Constant("PIPELINE_RUN", cel.StringType, types.String(typePipelineRun)),
		cel.Constant("TASK_RUN", cel.StringType, types.String(typeTaskRun)),
		cel.Variable("parent", cel.StringType),
		cel.Variable("result_name", cel.StringType),
		cel.Variable("name", cel.StringType),
		cel.Variable("data_type", cel.StringType),
		cel.Variable("data", cel.AnyType),
	)
}
