package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	"github.com/spf13/pflag"
	"github.com/tektoncd/results/pkg/cli/cmd"
	"github.com/tektoncd/results/pkg/cli/common"
)

type options struct {
	source string
	target string
	kind   string
}

func parseArgs() (*options, error) {
	opts := &options{}
	cwd, _ := os.Getwd()
	flags := pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
	flags.StringVar(&opts.source, "root", cwd, "Path to project root")
	flags.StringVar(&opts.target, "target", "/tmp", "Target path for generated yaml files")
	flags.StringVar(&opts.kind, "kind", "markdown", "Kind of docs to generate (supported: man, markdown)")
	err := flags.Parse(os.Args[1:])
	return opts, err
}

func generateCliYaml(opts *options) error {
	tp := &common.ResultsParams{}
	root := cmd.Root(tp)
	disableFlagsInUseLine(root)

	root.DisableAutoGenTag = true
	switch opts.kind {
	case "markdown":
		return GenMarkdownTree(root, opts.target)
	case "man":
		header := &doc.GenManHeader{
			Title:   "TKN-RESULTS",
			Section: "1",
			Source:  "Tekton Results CLI",
		}
		return doc.GenManTree(root, header, opts.target)
	default:
		return fmt.Errorf("invalid docs kind : %s", opts.kind)
	}
}

func disableFlagsInUseLine(cmd *cobra.Command) {
	visitAll(cmd, func(ccmd *cobra.Command) {
		// do not add a `[flags]` to the end of the usage line.
		ccmd.DisableFlagsInUseLine = true
	})
}

// visitAll will traverse all commands from the root.
// This is different from the VisitAll of cobra.Command where only parents
// are checked.
func visitAll(root *cobra.Command, fn func(*cobra.Command)) {
	for _, cmd := range root.Commands() {
		visitAll(cmd, fn)
	}
	fn(root)
}

// GenMarkdownTree is the same as GenMarkdownTree, but
// with custom filePrepender and linkHandler.
func GenMarkdownTree(cmd *cobra.Command, dir string) error {
	identity := func(s string) string { return s }
	emptyStr := func(_ string) string { return "" }
	return doc.GenMarkdownTreeCustom(cmd, dir, emptyStr, identity)
}

func main() {
	opts, err := parseArgs()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
	}

	fmt.Printf("Project root: %s\n", opts.source)
	fmt.Printf("Generating yaml files into %s\n", opts.target)
	if err := generateCliYaml(opts); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate yaml files: %s\n", err.Error())
	}
}
