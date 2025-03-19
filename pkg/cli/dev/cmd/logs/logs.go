// Copyright 2023 The Tekton Authors
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

package logs

import (
	"github.com/spf13/cobra"
	"github.com/tektoncd/results/pkg/cli/dev/flags"
)

// Command returns a cobra command for `logs` sub commands
func Command(params *flags.Params) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Commands for finding and retrieving logs",
		Annotations: map[string]string{
			"commandType": "main",
		},
	}

	cmd.AddCommand(ListLogsCommand(params), GetLogCommand(params))

	return cmd
}
