package options

import (
	"github.com/tektoncd/results/pkg/cli/client"
	"github.com/tektoncd/results/pkg/cli/common"
)

var _ common.FilterOptions = (*ListOptions)(nil)

// ListOptions holds the options for listing resources
type ListOptions struct {
	Client        *client.RESTClient
	Limit         int32
	AllNamespaces bool
	Label         string
	PipelineRun   string
	SinglePage    bool
	ResourceName  string
	ResourceType  string
}

// GetLabel implements FilterOptions interface
func (o *ListOptions) GetLabel() string {
	return o.Label
}

// GetResourceName implements FilterOptions interface
func (o *ListOptions) GetResourceName() string {
	return o.ResourceName
}

// GetPipelineRun implements FilterOptions interface
func (o *ListOptions) GetPipelineRun() string {
	return o.PipelineRun
}

// GetResourceType implements FilterOptions interface
func (o *ListOptions) GetResourceType() string {
	return o.ResourceType
}

// GetUID implements FilterOptions interface
func (o *ListOptions) GetUID() string {
	return ""
}

// SelectsExactMatch implements FilterOptions interface.
// List uses substring matching to support partial name searches.
func (o *ListOptions) SelectsExactMatch() bool {
	return false
}
