package flags

import (
	"github.com/spf13/cobra"
	"github.com/tektoncd/cli/pkg/formatted"
	"github.com/tektoncd/results/pkg/cli/common"
)

const (
	kubeConfig            = "kubeconfig"
	context               = "context"
	namespace             = "namespace"
	host                  = "host"
	token                 = "token"
	apiPath               = "api-path"
	insecureSkipTLSVerify = "insecure-skip-tls-verify"
)

// ResultsOptions all global Results options
type ResultsOptions struct {
	KubeConfig, Context, Namespace, Host, Token, APIPath string
	InsecureSkipTLSVerify                                bool
}

// AddResultsOptions amends the provided command by adding flags required to initialize a cli.Param.
// It adds persistent flags for kubeconfig, context, namespace, host, token, API path, and TLS verification.
//
// Parameters:
//   - cmd: A pointer to a cobra.Command to which the flags will be added.
//
// This function does not return any value.
func AddResultsOptions(cmd *cobra.Command) {
	cmd.PersistentFlags().StringP(
		kubeConfig, "k", "",
		"kubectl config file (default: $HOME/.kube/config)")

	cmd.PersistentFlags().StringP(
		context, "c", "",
		"name of the kubeconfig context to use (default: kubectl config current-context)")

	cmd.PersistentFlags().StringP(
		namespace, "n", "",
		"namespace to use (default: from $KUBECONFIG)")
	_ = cmd.RegisterFlagCompletionFunc(namespace,
		func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			return formatted.BaseCompletion("namespace", args)
		},
	)

	cmd.PersistentFlags().StringP(
		host, "", "",
		"host to use (default: value provided in config set command)")

	cmd.PersistentFlags().StringP(
		token, "", "",
		"bearer token to use (default: value provided in config set command)")

	cmd.PersistentFlags().StringP(
		apiPath, "", "",
		"api path to use (default: value provided in config set command)")

	cmd.PersistentFlags().BoolP(
		insecureSkipTLSVerify, "", false,
		"skip server's certificate validation for requests (default: false)")

}

// GetResultsOptions retrieves the global Results Options that are not passed to subcommands.
// It extracts flag values from the provided command and returns them as a ResultsOptions struct.
//
// Parameters:
//   - cmd: A pointer to a cobra.Command from which to retrieve flag values.
//
// Returns:
//   - ResultsOptions: A struct containing the extracted flag values for kubeconfig, context,
//     namespace, host, token, API path, and TLS verification settings.
func GetResultsOptions(cmd *cobra.Command) ResultsOptions {
	kcPath, _ := cmd.Flags().GetString(kubeConfig)
	kubeContext, _ := cmd.Flags().GetString(context)
	ns, _ := cmd.Flags().GetString(namespace)
	h, _ := cmd.Flags().GetString(host)
	t, _ := cmd.Flags().GetString(token)
	ap, _ := cmd.Flags().GetString(apiPath)
	skipTLSVerify, _ := cmd.Flags().GetBool(insecureSkipTLSVerify)
	return ResultsOptions{
		KubeConfig:            kcPath,
		Context:               kubeContext,
		Namespace:             ns,
		Host:                  h,
		Token:                 t,
		APIPath:               ap,
		InsecureSkipTLSVerify: skipTLSVerify,
	}
}

// InitParams initializes cli.Params based on flags defined in the command.
//
// This function retrieves flag values from the provided cobra.Command and sets
// the corresponding values in the common.Params object. It handles flags for
// kubeconfig, context, namespace, host, token, API path, and TLS verification.
//
// Parameters:
//   - p: A common.Params object to be initialized with flag values.
//   - cmd: A *cobra.Command object containing the flags to be processed.
//
// Returns:
//   - An error if any flag retrieval operation fails, nil otherwise.
//
// Note: This function uses cmd.Flags() instead of cmd.PersistentFlags() to
// access flags, which may break symmetry with AddResultsOptions. This is because
// it could be a subcommand trying to access flags defined by the parent command.
func InitParams(p common.Params, cmd *cobra.Command) error {
	// First set kubeconfig and context
	kcPath, err := cmd.Flags().GetString(kubeConfig)
	if err != nil {
		return err
	}
	p.SetKubeConfigPath(kcPath)

	kubeContext, err := cmd.Flags().GetString(context)
	if err != nil {
		return err
	}
	p.SetKubeContext(kubeContext)

	// Then set namespace, which will use the kubeconfig and context if needed
	ns, err := cmd.Flags().GetString(namespace)
	if err != nil {
		return err
	}
	p.SetNamespace(ns)

	// Set other flags
	h, err := cmd.Flags().GetString(host)
	if err != nil {
		return err
	}
	if h != "" {
		p.SetHost(h)
	}

	t, err := cmd.Flags().GetString(token)
	if err != nil {
		return err
	}
	if t != "" {
		p.SetToken(t)
	}

	ap, err := cmd.Flags().GetString(apiPath)
	if err != nil {
		return err
	}
	if ap != "" {
		p.SetAPIPath(ap)
	}

	skipTLSVerify, err := cmd.Flags().GetBool(insecureSkipTLSVerify)
	if err != nil {
		return err
	}
	p.SetSkipTLSVerify(skipTLSVerify)

	return nil
}

// AnyResultsFlagChanged checks if any of the Results flags (host, token, api-path, insecure-skip-tls-verify)
// have been changed from their default values.
//
// Parameters:
//   - cmd: A pointer to a cobra.Command to check the flags.
//
// Returns:
//   - bool: true if any of the Results flags have been changed, false otherwise.
func AnyResultsFlagChanged(cmd *cobra.Command) bool {
	return cmd.Flags().Changed(host) ||
		cmd.Flags().Changed(token) ||
		cmd.Flags().Changed(apiPath) ||
		cmd.Flags().Changed(insecureSkipTLSVerify)
}
