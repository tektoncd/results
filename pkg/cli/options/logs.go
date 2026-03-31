package options

import (
	"github.com/tektoncd/results/pkg/cli/client"
	"github.com/tektoncd/results/pkg/cli/common"
)

var _ common.FilterOptions = (*LogsOptions)(nil)

// LogsOptions contains options for fetching logs for a resource.
type LogsOptions struct {
	Client       *client.RESTClient
	UID          string
	ResourceType string
	ResourceName string
}

// GetLabel implements FilterOptions interface
func (o *LogsOptions) GetLabel() string {
	return "" // Label field is not relevant in the logs commands
}

// GetResourceName implements FilterOptions interface
func (o *LogsOptions) GetResourceName() string {
	return o.ResourceName
}

// GetPipelineRun implements FilterOptions interface
func (o *LogsOptions) GetPipelineRun() string {
	return "" // PipelineRun field is not relevant in the logs commands
}

// GetResourceType implements FilterOptions interface
func (o *LogsOptions) GetResourceType() string {
	return o.ResourceType
}

// GetUID implements FilterOptions interface
func (o *LogsOptions) GetUID() string {
	return o.UID
}

// SelectsExactMatch implements FilterOptions interface.
// Logs uses exact match for faster server-side filtering.
func (o *LogsOptions) SelectsExactMatch() bool {
	return true
}
