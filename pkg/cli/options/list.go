package options

import "github.com/tektoncd/results/pkg/cli/client"

// ListOptions holds the options for listing resources
type ListOptions struct {
	Client        *client.RESTClient
	Limit         int32
	AllNamespaces bool
	Label         string
	PipelineRun   string
	SinglePage    bool
	ResourceName  string
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
