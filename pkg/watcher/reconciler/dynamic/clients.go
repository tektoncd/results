package dynamic

import (
	"context"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/client/clientset/versioned/typed/pipeline/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ObjectClient is a shim around generated k8s clients to handle objects in
// type agnostic ways.
// This might be able to be replaced with generics later?
type ObjectClient interface {
	Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) error
}

// TaskRunClient implements the dynamic ObjectClient for TaskRuns.
type TaskRunClient struct {
	pipelinev1.TaskRunInterface
}

// Patch patches TaskRun k8s resource
func (c *TaskRunClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) error {
	_, err := c.TaskRunInterface.Patch(ctx, name, pt, data, opts, subresources...)
	return err
}

// PipelineRunClient implements the dynamic ObjectClient for TaskRuns.
type PipelineRunClient struct {
	pipelinev1.PipelineRunInterface
}

// Patch patches pipelineRun Kubernetes resource.
func (c *PipelineRunClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) error {
	_, err := c.PipelineRunInterface.Patch(ctx, name, pt, data, opts, subresources...)
	return err
}
