// Copyright 2026 The Tekton Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Command tkn-results is the Tekton Results CLI entrypoint.
package main

import (
	"fmt"
	"os"

	"github.com/tektoncd/results/pkg/cli/cmd"
	"github.com/tektoncd/results/pkg/cli/common"
)

func main() {
	p := &common.ResultsParams{}
	root := cmd.Root(p)
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
