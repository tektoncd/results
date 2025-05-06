package options

import "github.com/tektoncd/results/pkg/cli/client"

// DescribeOptions contains options for describing a resource.
type DescribeOptions struct {
	Client       *client.RESTClient
	UID          string
	ResourceType string
	ResourceName string
}

// GetLabel implements FilterOptions interface
func (o *DescribeOptions) GetLabel() string {
	return "" // Label field is not relevant in the describe commands
}

// GetResourceName implements FilterOptions interface
func (o *DescribeOptions) GetResourceName() string {
	return o.ResourceName
}

// GetPipelineRun implements FilterOptions interface
func (o *DescribeOptions) GetPipelineRun() string {
	return "" // PipelineRun field is not relevant in the describe commands
}

// GetResourceType implements FilterOptions interface
func (o *DescribeOptions) GetResourceType() string {
	return o.ResourceType
}

// GetUID implements FilterOptions interface
func (o *DescribeOptions) GetUID() string {
	return o.UID
}
