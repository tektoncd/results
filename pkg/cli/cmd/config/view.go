package config

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/tektoncd/results/pkg/cli/common"
	"github.com/tektoncd/results/pkg/cli/config"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/cli-runtime/pkg/printers"
	"k8s.io/client-go/kubernetes/scheme"
)

// ViewOptions contains the configuration for the view command.
type ViewOptions struct {
	Config      config.Config
	PrintFlags  *genericclioptions.PrintFlags
	PrinterFunc printers.ResourcePrinterFunc
	IOStreams   *genericiooptions.IOStreams
}

// viewCommand creates a new cobra.Command for viewing Tekton Results configuration.
//
// Parameters:
//   - p: common.Params containing shared parameters for the CLI.
//
// Returns:
//   - *cobra.Command: A configured cobra.Command ready to be added to the CLI.
func viewCommand(p common.Params) *cobra.Command {
	ios := &genericiooptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	opts := &ViewOptions{
		PrintFlags: genericclioptions.NewPrintFlags("").WithTypeSetter(scheme.Scheme).WithDefaultOutput("yaml"),
		IOStreams:  ios,
	}
	c := &cobra.Command{
		Use:   "view",
		Short: "Display current CLI configuration",
		Long: `Display the current configuration settings for the Tekton Results CLI.

This command shows all configured settings including:
- API server endpoint
- Authentication token
- Cluster context and namespace
- TLS verification settings

The configuration is displayed in YAML format.

Examples:
  # View current configuration
  tkn-results config view`,
		PreRunE: func(_ *cobra.Command, _ []string) error {
			printer, err := opts.PrintFlags.ToPrinter()
			if err != nil {
				return err
			}
			opts.PrinterFunc = printer.PrintObj

			return nil
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			var err error
			opts.Config, err = config.NewConfig(p)
			if err != nil {
				return err
			}
			return opts.PrinterFunc(opts.Config.GetObject(), opts.IOStreams.Out)
		},
	}
	return c
}
