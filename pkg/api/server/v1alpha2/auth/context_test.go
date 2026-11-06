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

import (
	"context"
	"testing"
)

func TestResourceAttributesFrom(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want ResourceAttributes
	}{
		{
			name: "defaults when unset",
			ctx:  context.Background(),
			want: ResourceAttributes{Group: resultsAPIGroup, Version: resultsAPIVersion},
		},
		{
			name: "preserves core API group and object identity",
			ctx: WithResourceAttributes(context.Background(), ResourceAttributes{
				Group: "", Version: "v1", Name: "example", Subresource: "log",
			}),
			want: ResourceAttributes{Group: "", Version: "v1", Name: "example", Subresource: "log"},
		},
		{
			name: "preserves named API group",
			ctx: WithResourceAttributes(context.Background(), ResourceAttributes{
				Group: "tekton.dev", Version: "v1",
			}),
			want: ResourceAttributes{Group: "tekton.dev", Version: "v1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResourceAttributesFrom(tt.ctx); got != tt.want {
				t.Fatalf("ResourceAttributesFrom() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
