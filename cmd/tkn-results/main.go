package main

import (
	"context"
	"github.com/tektoncd/results/pkg/cli/cmd"
)

func main() {
	cmd.Root().ExecuteContext(context.Background())
}
