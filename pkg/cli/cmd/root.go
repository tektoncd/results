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

	c.PersistentFlags().StringP("addr", "a", "", "[To be deprecated] Result API server address. If not specified, tkn-result would port-forward to service/tekton-results-api-service automatically")
	c.PersistentFlags().StringP("authtoken", "t", "", "[To be deprecated] authorization bearer token to use for authenticated requests")
	c.PersistentFlags().String("sa", "", "[To be deprecated] ServiceAccount to use instead of token for authorization and authentication")
	c.PersistentFlags().String("sa-ns", "", "[To be deprecated] ServiceAccount Namespace, if not given, it will be taken from current context")
	c.PersistentFlags().Bool("portforward", true, "[To be deprecated] enable auto portforwarding to tekton-results-api-service, when addr is set and portforward is true, tkn-results will portforward tekton-results-api-service automatically")
	c.PersistentFlags().Bool("insecure", false, "[To be deprecated] determines whether to run insecure GRPC tls request")
	c.PersistentFlags().Bool("v1alpha2", false, "[To be deprecated] use v1alpha2 API for get log command")

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
