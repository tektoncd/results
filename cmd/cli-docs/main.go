package main

import (
	"log"

	"github.com/spf13/cobra/doc"
	"github.com/tektoncd/results/pkg/cli/cmd"
)

func main() {
	if err := doc.GenMarkdownTree(cmd.Root(), "./docs/cli/"); err != nil {
		log.Fatal(err)
	}
}
