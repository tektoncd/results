// Package cmd provides the root command and subcommands for the Results CLI.
package cmd

import (
	_ "embed"
	"flag"
	"fmt"

	"github.com/jonboulle/clockwork"

	"github.com/tektoncd/results/pkg/cli/cmd/taskrun"

	"github.com/tektoncd/results/pkg/cli/cmd/pipelinerun"

	"github.com/tektoncd/results/pkg/cli/cmd/config"
	"github.com/tektoncd/results/pkg/cli/common"

	"github.com/tektoncd/results/pkg/cli/dev/client"
	"github.com/tektoncd/results/pkg/cli/dev/cmd/logs"
	"github.com/tektoncd/results/pkg/cli/dev/cmd/records"
	"github.com/tektoncd/results/pkg/cli/dev/cmd/result"
	devConfig "github.com/tektoncd/results/pkg/cli/dev/config"
	"github.com/tektoncd/results/pkg/cli/dev/flags"
	"github.com/tektoncd/results/pkg/cli/dev/portforward"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	// TODO: Dynamically discover other protos to allow custom record printing.
	_ "github.com/tektoncd/results/proto/pipeline/v1/pipeline_go_proto"
)

var (
	//go:embed help.txt
	help string
)

// Root returns a cobra command for `tkn-results` root sub commands
func Root(p common.Params) *cobra.Command {
	// params values are populated in the preRun Handler
	params := &flags.Params{}
	var portForwardCloseChan chan struct{}
	c := &cobra.Command{
		Use:   "tkn-results",
		Short: "Tekton Results CLI",
		Long:  help,
		PersistentPreRunE: func(c *cobra.Command, _ []string) error {
			// this will only run when older commands is being used
			return persistentPreRunHandler(c, params, &portForwardCloseChan)
		},
		PersistentPostRun: func(_ *cobra.Command, _ []string) {
			if portForwardCloseChan != nil {
				close(portForwardCloseChan)
			}
		},
		Annotations: map[string]string{
			"commandType": "main",
		},
	}

	c.PersistentFlags().StringP("addr", "a", "", "[DEPRECATED] Result API server address. Use 'config set --host=<host>' instead")
	_ = c.PersistentFlags().MarkDeprecated("addr", "use 'config set --host=<host>' to configure the API server address")
	c.PersistentFlags().StringP("authtoken", "t", "", "[DEPRECATED] authorization bearer token. Use 'config set --token=<token>' instead")
	_ = c.PersistentFlags().MarkDeprecated("authtoken", "use 'config set --token=<token>' to configure authentication")
	c.PersistentFlags().String("sa", "", "[DEPRECATED] ServiceAccount for authorization. Use 'config set' instead")
	_ = c.PersistentFlags().MarkDeprecated("sa", "use 'config set' for service account-based authentication")
	c.PersistentFlags().String("sa-ns", "", "[DEPRECATED] ServiceAccount Namespace. Use 'config set' instead")
	_ = c.PersistentFlags().MarkDeprecated("sa-ns", "use 'config set' for service account configuration")
	c.PersistentFlags().Bool("portforward", true, "[DEPRECATED] enable auto portforwarding. Use 'config set' instead")
	_ = c.PersistentFlags().MarkDeprecated("portforward", "use 'config set' to configure portforwarding behavior")
	c.PersistentFlags().Bool("insecure", false, "[DEPRECATED] determines whether to run insecure GRPC tls request. Use 'config set' instead")
	_ = c.PersistentFlags().MarkDeprecated("insecure", "use 'config set --insecure' to configure TLS settings")
	c.PersistentFlags().Bool("v1alpha2", false, "[DEPRECATED] use v1alpha2 API. This flag is no longer needed")
	_ = c.PersistentFlags().MarkDeprecated("v1alpha2", "v1alpha2 API support has been deprecated and this flag has no effect")

	c.AddCommand(
		// Commands to be deprecated
		result.Command(params),
		records.Command(params),
		logs.Command(params),
		// new commands
		config.Command(p),
		pipelinerun.Command(p),
		taskrun.Command(p),
	)

	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	err := viper.BindPFlags(c.PersistentFlags())
	if err != nil {
		return nil
	}
	cobra.OnInitialize(devConfig.Init)

	return c
}

func persistentPreRunHandler(c *cobra.Command, params *flags.Params, portForwardCloseChan *chan struct{}) error {
	var overrideAPIAdr string

	// Prepare to port-forward if addr config is not set
	if cfg := devConfig.GetConfig(); cfg.Portforward && cfg.Address == "" {
		portForward, err := portforward.NewPortForward()
		if err != nil {
			return err
		}
		// Pick a usable port on localhost for port-forwarding
		port, err := portforward.PickFreePort()
		if err != nil {
			return err
		}
		overrideAPIAdr = fmt.Sprintf("localhost:%d", port)
		*portForwardCloseChan = make(chan struct{})
		if err = portForward.ForwardPortBackground(*portForwardCloseChan, port); err != nil {
			return err
		}
	}

	// Initialize API clients
	apiClient, err := client.DefaultResultsClient(c.Context(), overrideAPIAdr)
	if err != nil {
		return err
	}
	params.ResultsClient = apiClient

	logClient, err := client.DefaultLogsClient(c.Context(), overrideAPIAdr)
	if err != nil {
		return err
	}
	params.LogsClient = logClient

	pluginLogsClient, err := client.DefaultPluginLogsClient(c.Context(), overrideAPIAdr)
	if err != nil {
		return err
	}
	params.PluginLogsClient = pluginLogsClient

	params.Clock = clockwork.NewRealClock()

	return nil
}
