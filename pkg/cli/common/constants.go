package common

const (
	// ResourceTypeTaskRun is the string representation of a TaskRun resource
	ResourceTypeTaskRun = "taskrun"
	// ResourceTypePipelineRun is the string representation of a PipelineRun resource
	ResourceTypePipelineRun = "pipelinerun"
	// ListFields is the string used to fetch partial response
	//  for the list command in the API calls
	ListFields = "records.name,records.uid,records.create_time,records.update_time,records.data.value.metadata,records.data.value.status,next_page_token"
	// NameAndUIDField is the string used to fetch only name and
	// UID in an API call
	NameAndUIDField = "records.name,records.uid"
)
