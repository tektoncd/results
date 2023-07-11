package main

import (
	"context"
	"github.com/tektoncd/results/tools/tkn-results/cmd"
)

func main() {
	cmd.Root().ExecuteContext(context.Background())
}
