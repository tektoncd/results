package common

import (
	"fmt"
	"strings"
)

// ValidateLabels validates the format of the provided labels string.
// Labels should be in the format "key=value" or "key=value,key2=value2".
// Returns an error if the format is invalid.
func ValidateLabels(labels string) error {
	labelPairs := strings.Split(labels, ",")
	for _, pair := range labelPairs {
		parts := strings.Split(strings.TrimSpace(pair), "=")
		if len(parts) != 2 {
			return fmt.Errorf("invalid label format: %s. Expected format: key=value", pair)
		}

		// Check for whitespace in key before trimming
		if strings.ContainsAny(parts[0], " \t") {
			return fmt.Errorf("label key cannot contain whitespace: %s", parts[0])
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if key == "" {
			return fmt.Errorf("label key cannot be empty in pair: %s", pair)
		}
		if value == "" {
			return fmt.Errorf("label value cannot be empty in pair: %s", pair)
		}
	}

	return nil
}

// FilterOptions defines the interface for filter options
type FilterOptions interface {
	GetLabel() string
	GetResourceName() string
	GetPipelineRun() string
	GetResourceType() string
	GetUID() string
}

// BuildFilterString constructs the filter string for the ListRecordsRequest
func BuildFilterString(opts FilterOptions) string {
	const (
		contains = "data.metadata.%s.contains(\"%s\")"
		equal    = "data.metadata.%s[\"%s\"]==\"%s\""
		dataType = "data_type==\"%s\""
	)

	var filters []string

	switch opts.GetResourceType() {
	case ResourceTypePipelineRun:
		// Add data type filter for both v1 and v1beta1 PipelineRuns
		filters = append(filters, fmt.Sprintf(`(%s || %s)`,
			fmt.Sprintf(dataType, "tekton.dev/v1.PipelineRun"),
			fmt.Sprintf(dataType, "tekton.dev/v1beta1.PipelineRun")))
	case ResourceTypeTaskRun:
		// Add data type filter for both v1 and v1beta1 TaskRuns
		filters = append(filters, fmt.Sprintf(`(%s || %s)`,
			fmt.Sprintf(dataType, "tekton.dev/v1.TaskRun"),
			fmt.Sprintf(dataType, "tekton.dev/v1beta1.TaskRun")))
	}

	// Handle label filters
	if opts.GetLabel() != "" {
		// Split by comma to get individual label pairs
		labelPairs := strings.Split(opts.GetLabel(), ",")
		for _, pair := range labelPairs {
			// Split each pair by = to get key and value
			parts := strings.Split(strings.TrimSpace(pair), "=")
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				filters = append(filters, fmt.Sprintf(equal, "labels", key, value))
			}
		}
	}

	// Handle pipeline name filter
	if opts.GetResourceName() != "" {
		filters = append(filters, fmt.Sprintf(contains, "name", opts.GetResourceName()))
	}

	// Handle UID filter
	if opts.GetUID() != "" {
		filters = append(filters, fmt.Sprintf(contains, "uid", opts.GetUID()))
	}

	// Add PipelineRun filter if provided
	if opts.GetPipelineRun() != "" {
		filters = append(filters, fmt.Sprintf(`data.metadata.labels['tekton.dev/pipelineRun'] == '%s'`, opts.GetPipelineRun()))
	}

	return strings.Join(filters, " && ")
}
