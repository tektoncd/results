// Copyright 2024 The Tekton Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package client provides utilities for interacting with watcher reconciler clients.
package client

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

// PipelineRunClient implements the dynamic ObjectClient for PipelineRuns.
type PipelineRunClient struct {
	pipelinev1.PipelineRunInterface
}

// Patch patches pipelineRun Kubernetes resource.
func (c *PipelineRunClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) error {
	_, err := c.PipelineRunInterface.Patch(ctx, name, pt, data, opts, subresources...)
	return err
}
