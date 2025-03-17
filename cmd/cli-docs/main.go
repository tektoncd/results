package main

import (
	"log"

	"github.com/tektoncd/results/pkg/cli/cmd"

	"github.com/spf13/cobra/doc"
)

func main() {
	if err := doc.GenMarkdownTree(cmd.Root(), "./docs/cli/"); err != nil {
		log.Fatal(err)
	}
}
