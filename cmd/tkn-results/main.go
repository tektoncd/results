package main

import (
	"context"
	"os"

	"github.com/tektoncd/results/pkg/cli/cmd"
)

func main() {
	err := cmd.Root().ExecuteContext(context.Background())
	if err != nil {
		os.Exit(1)
	}
}
