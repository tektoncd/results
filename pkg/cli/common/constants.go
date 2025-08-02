package common

const (
	// ResourceTypeTaskRun is the string representation of a TaskRun resource
	ResourceTypeTaskRun = "taskrun"
	// ResourceTypePipelineRun is the string representation of a PipelineRun resource
	ResourceTypePipelineRun = "pipelinerun"
	// ListFields is the string used to fetch partial response
	//  for the list command in the API calls
	ListFields = "records.name,records.uid,records.create_time,records.update_time,records.data.value.metadata,records.data.value.status,next_page_token"
	// NameUIDAndDataField is the string used to fetch name, UID and Data
	NameUIDAndDataField = "records.name,records.uid,records.data.value"
	// AllNamespacesResultsParent is the parent path used for listing results across all namespaces.
	AllNamespacesResultsParent = "-/results/-"
)
