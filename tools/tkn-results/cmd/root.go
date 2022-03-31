package cmd

import (
	_ "embed"
	"flag"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/tektoncd/results/tools/tkn-results/cmd/records"
	"github.com/tektoncd/results/tools/tkn-results/internal/client"
	"github.com/tektoncd/results/tools/tkn-results/internal/flags"

	// TODO: Dynamically discover other protos to allow custom record printing.
	_ "github.com/tektoncd/results/proto/pipeline/v1beta1/pipeline_go_proto"
)

var (
	//go:embed help.txt
	help string
)

func Root() *cobra.Command {
	params := &flags.Params{}

	cmd := &cobra.Command{
		Use:   "tkn-results",
		Short: "tkn CLI plugin for Tekton Results API",
		Long:  help,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			apiClient, err := client.DefaultClient(cmd.Context())

			if err != nil {
				return err
			}

			params.Client = apiClient

			return nil
		},
	}

	cmd.PersistentFlags().StringP("addr", "a", "", "Result API server address")
	cmd.PersistentFlags().StringP("authtoken", "t", "", "authorization bearer token to use for authenticated requests")

	cmd.AddCommand(ListCommand(params), records.Command(params))

	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	viper.BindPFlags(cmd.PersistentFlags())

	return cmd
}
