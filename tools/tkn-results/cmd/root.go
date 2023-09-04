package cmd

import (
	_ "embed"
	"flag"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/tektoncd/results/tools/tkn-results/cmd/records"
	"github.com/tektoncd/results/tools/tkn-results/internal/client"
	"github.com/tektoncd/results/tools/tkn-results/internal/config"
	"github.com/tektoncd/results/tools/tkn-results/internal/flags"
	"github.com/tektoncd/results/tools/tkn-results/internal/portforward"

	// TODO: Dynamically discover other protos to allow custom record printing.
	_ "github.com/tektoncd/results/proto/pipeline/v1beta1/pipeline_go_proto"
)

var (
	//go:embed help.txt
	help string
)

func Root() *cobra.Command {
	params := &flags.Params{}
	var portForwardCloseChan chan struct{}
	cmd := &cobra.Command{
		Use:   "tkn-results",
		Short: "tkn CLI plugin for Tekton Results API",
		Long:  help,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			var overrideApiAdr string

			// Prepare to port-forward if addr config is not set
			if cfg := config.GetConfig(); cfg.Portforward && cfg.Address == "" {
				portForward, err := portforward.NewPortForward()
				if err != nil {
					return err
				}
				// Pick a usable port on localhost for port-forwarding
				port, err := portforward.PickFreePort()
				if err != nil {
					return err
				}
				overrideApiAdr = fmt.Sprintf("localhost:%d", port)
				portForwardCloseChan = make(chan struct{})
				if err = portForward.ForwardPortBackground(portForwardCloseChan, port); err != nil {
					return err
				}
			}

			apiClient, err := client.DefaultResultsClient(cmd.Context(), overrideApiAdr)

			if err != nil {
				return err
			}

			params.ResultsClient = apiClient

			logClient, err := client.DefaultLogsClient(cmd.Context(), overrideApiAdr)

			if err != nil {
				return err
			}

			params.LogsClient = logClient

			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if portForwardCloseChan != nil {
				close(portForwardCloseChan)
			}
		},
		Annotations: map[string]string{
			"commandType": "main",
		},
	}

	cmd.PersistentFlags().StringP("addr", "a", "", "Result API server address. If not specified, tkn-result would port-forward to service/tekton-results-api-service automatically")
	cmd.PersistentFlags().StringP("authtoken", "t", "", "authorization bearer token to use for authenticated requests")
	cmd.PersistentFlags().String("sa", "", "ServiceAccount to use instead of token for authorization and authentication")
	cmd.PersistentFlags().String("sa-ns", "", "ServiceAccount Namespace, if not given, it will be taken from current context")
	cmd.PersistentFlags().Bool("portforward", true, "enable auto portforwarding to tekton-results-api-service, when addr is set and portforward is true, tkn-results will portforward tekton-results-api-service automatically")
	cmd.PersistentFlags().Bool("insecure", false, "determines whether to run insecure GRPC tls request")

	cmd.AddCommand(ListCommand(params), records.Command(params))

	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	viper.BindPFlags(cmd.PersistentFlags())
	cobra.OnInitialize(config.Init)

	return cmd
}
