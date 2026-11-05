// Copyright 2026 The Tekton Authors
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

package auth

import "context"

type resourceAttributesKey struct{}

// ResourceAttributes identifies the Kubernetes resource details that are not
// part of Checker.Check's legacy namespace/resource/verb parameters.
type ResourceAttributes struct {
	Group       string
	Version     string
	Name        string
	Subresource string
}

// WithResourceAttributes attaches the requested resource details to ctx so
// authorize can include them in SubjectAccessReview attributes and authz keys.
// The K8s-style HTTP path uses this; the Results gRPC path leaves it unset
// (defaults to results.tekton.dev with other fields empty).
func WithResourceAttributes(ctx context.Context, attrs ResourceAttributes) context.Context {
	return context.WithValue(ctx, resourceAttributesKey{}, attrs)
}

// ResourceAttributesFrom returns the resource details from ctx, or the Results
// defaults when they are unset.
func ResourceAttributesFrom(ctx context.Context) ResourceAttributes {
	if v, ok := ctx.Value(resourceAttributesKey{}).(ResourceAttributes); ok {
		return v
	}
	return ResourceAttributes{Group: resultsAPIGroup, Version: resultsAPIVersion}
}
