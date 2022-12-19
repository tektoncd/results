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

package reconciler

import (
	"time"

	"k8s.io/apimachinery/pkg/labels"
)

// Config defines shared reconciler configuration options.
type Config struct {
	// Configures whether Tekton CRD objects should be updated with Result
	// annotations during reconcile. Useful to enable for dry run modes.
	DisableAnnotationUpdate bool

	// CompletedResourceGracePeriod is the time to wait before deleting completed resources.
	// 0 implies the duration
	CompletedResourceGracePeriod time.Duration

	// Label selector to match resources against in order to determine
	// whether completed resources are eligible for deletion. The default
	// value is labels.Everything() which matches any resource.
	labelSelector labels.Selector
}

// GetDisableAnnotationupdate returns whether annotation updates should be
// disabled. This is safe to call for missing configs.
func (c *Config) GetDisableAnnotationUpdate() bool {
	if c == nil {
		return false
	}
	return c.DisableAnnotationUpdate
}

// GetCompletedResourceGracePeriod returns the grace period to wait for
// deleting Run objects.
// If value < 0, objects will be deleted immediately.
// If value = 0 (or not explicitly set), then objects will not be deleted.
// If value > 0, objects will be deleted with a grace period option of the
// duration.
func (c *Config) GetCompletedResourceGracePeriod() time.Duration {
	if c == nil {
		return 0
	}
	return c.CompletedResourceGracePeriod
}

// GetLabelSelector returns the label selector to match resources against in
// order to determine whether they're eligible for deletion. If no selector was
// configured via the SetLabelSelector method, returns a selector that always
// matches any resource.
func (c *Config) GetLabelSelector() labels.Selector {
	if c.labelSelector == nil {
		return labels.Everything()
	}
	return c.labelSelector
}

// SetLabelSelector sets a label selector to match resources against in order to
// determine whether they're eligible for deletion. The syntax obeys the same
// format accepted by list operations peformed on the Kubernetes API server.
func (c *Config) SetLabelSelector(selector string) error {
	parsedSelector, err := labels.Parse(selector)
	if err != nil {
		return err
	}
	c.labelSelector = parsedSelector
	return nil
}
