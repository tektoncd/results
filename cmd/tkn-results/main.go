package main

import (
	"context"
	"os"

	"github.com/tektoncd/results/pkg/cli/common"

	"github.com/tektoncd/results/pkg/cli/cmd"
)

// Creates a new ResultsParams struct.
// Executes the root command with the ResultsParams and a background context.
func main() {
	tp := &common.ResultsParams{}
	err := cmd.Root(tp).ExecuteContext(context.Background())
	if err != nil {
		os.Exit(1)
	}
}
