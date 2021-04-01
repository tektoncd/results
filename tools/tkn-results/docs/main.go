package main

import (
	"log"

	"github.com/spf13/cobra/doc"
	"github.com/tektoncd/results/tools/tkn-results/cmd"
)

func main() {
	if err := doc.GenMarkdownTree(cmd.RootCmd, "./"); err != nil {
		log.Fatal(err)
	}
}
