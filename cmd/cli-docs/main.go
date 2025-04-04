package main

import (
	"log"

	"github.com/tektoncd/results/pkg/cli/common"

	"github.com/tektoncd/results/pkg/cli/cmd"

	"github.com/spf13/cobra/doc"
)

func main() {
	tp := &common.ResultsParams{}
	if err := doc.GenMarkdownTree(cmd.Root(tp), "./docs/cli/"); err != nil {
		log.Fatal(err)
	}
}
