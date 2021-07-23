// Copyright 2020 The Tekton Authors
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

package cel

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestParseFilter(t *testing.T) {
	env, err := NewEnv()
	if err != nil {
		t.Fatalf("NewEnv: %v", err)
	}

	t.Run("success", func(t *testing.T) {
		for _, s := range []string{
			"",
			"result",
			"result.id",
			`result.id == "1"`,
			`result.id == "1" || result.name == "2"`,
			`result.id.startsWith("tacocat")`,
		} {
			t.Run(s, func(t *testing.T) {
				if _, err := ParseFilter(env, s); err != nil {
					t.Fatal(err)
				}
			})
		}
	})

	t.Run("error", func(t *testing.T) {
		for _, s := range []string{
			"asdf",
			"result.id == 1", // string != int
			"result.ID",      // case sensitive
		} {
			t.Run(s, func(t *testing.T) {
				if p, err := ParseFilter(env, s); status.Code(err) != codes.InvalidArgument {
					t.Fatalf("expected error, got: (%v, %v)", p, err)
				}
			})
		}
	})
}
