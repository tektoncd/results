// Copyright 2021 The Tekton Authors
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

const (
	// ResourceResults - api results resource name
	ResourceResults = "results"
	// ResourceRecords - api record resource name
	ResourceRecords = "records"
	// ResourceLogs - api logs resource name
	ResourceLogs = "logs"
	// ResourceSummary - api summary
	ResourceSummary = "summary"

	// PermissionCreate - permission name to "create" resource
	PermissionCreate = "create"
	// PermissionGet - permission name to "get" resource
	PermissionGet = "get"
	// PermissionList - permission name to "list" resource
	PermissionList = "list"
	// PermissionDelete - permission name to "delete" resource
	PermissionDelete = "delete"
	// PermissionUpdate - permission name to "update" resource
	PermissionUpdate = "update"
)

// Checker handles authentication and authorization checks for an action on
// a resource.
type Checker interface {
	Check(ctx context.Context, parent, resource, verb string) error
}
