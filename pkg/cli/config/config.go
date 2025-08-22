package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strconv"
	"time"

	"github.com/tektoncd/results/pkg/cli/client"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/tektoncd/results/pkg/cli/common"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

// Constants defining various labels, names, and paths used in the Tekton Results configuration.
const (
	ExtensionName string = "tekton-results"
	Group         string = "results.tekton.dev"
	Version       string = "v1alpha2"
	Kind          string = "Client"
	Path          string = "apis"
)

// Config defines the interface for managing Tekton Results configuration.
type Config interface {
	Get() *client.Config
	GetObject() runtime.Object
	Set(prompt bool, p common.Params) error
	Reset(p common.Params) error
	Validate() error
}

type config struct {
	ConfigAccess clientcmd.ConfigAccess
	APIConfig    *api.Config
	RESTConfig   *rest.Config
	ClientConfig *client.Config
	Extension    *Extension
}

// NewConfig creates a new Config instance based on the provided parameters.
//
// It loads the kubeconfig, sets up the client configuration, and initializes
// the extension for Tekton Results.
//
// Parameters:
//   - p: common.Params containing configuration parameters such as kubeconfig path and context.
//
// Returns:
//   - Config: A new Config instance if successful.
//   - error: An error if any step in the configuration process fails.
func NewConfig(p common.Params) (Config, error) {
	kubeconfigPath := clientcmd.RecommendedHomeFile
	if p.KubeConfigPath() != "" {
		kubeconfigPath = p.KubeConfigPath()
	}
	// Load kubeConfig
	cc := getRawKubeConfigLoader(kubeconfigPath)
	ca := cc.ConfigAccess()
	ac, err := cc.RawConfig()
	if err != nil {
		return nil, err
	}

	// Get the desired context from user input
	ctx := p.KubeContext()
	if ctx == "" {
		// If no context is provided, use the current default context
		ctx = ac.CurrentContext
	}

	// Validate if the specified context exists
	if _, exists := ac.Contexts[ctx]; !exists {
		return nil, fmt.Errorf("context '%s' not found in kubeconfig", ctx)
	}

	// Create a REST config using the specified context
	overriddenConfig := clientcmd.NewNonInteractiveClientConfig(ac, ctx, &clientcmd.ConfigOverrides{}, ca)
	rc, err := overriddenConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	c := &config{
		ConfigAccess: ca,
		APIConfig:    &ac,
		RESTConfig:   rc,
	}
	if err := c.LoadExtension(p); err != nil {
		return nil, err
	}

	return c, c.LoadClientConfig()
}

// LoadClientConfig loads and configures the client configuration based on the current config state.
// It sets up the REST client configuration, including the GroupVersion, Host, APIPath, and authentication details.
// The function also configures TLS settings and timeout, and creates a common.Config with transport and URL information.
//
// Returns:
//   - error: An error if any step in the configuration process fails, nil otherwise.
func (c *config) LoadClientConfig() error {
	rc := rest.CopyConfig(c.RESTConfig)

	gv := c.Extension.TypeMeta.GroupVersionKind().GroupVersion()
	rc.GroupVersion = &gv

	if c.Extension.Host != "" {
		rc.Host = c.Extension.Host
	}

	if c.Extension.APIPath != "" {
		rc.APIPath = c.Extension.APIPath
	}

	if c.Extension.Token != "" {
		rc.BearerToken = c.Extension.Token
	}
	if i, err := strconv.ParseBool(c.Extension.InsecureSkipTLSVerify); err == nil {
		if i {
			rc.TLSClientConfig = rest.TLSClientConfig{}
		}
		rc.Insecure = i
	}

	if d, err := time.ParseDuration(c.Extension.Timeout); err != nil {
		rc.Timeout = d
	}

	tc, err := rc.TransportConfig()
	if err != nil {
		return err
	}

	rc.APIPath = path.Join(rc.APIPath, Path)
	u, p, err := rest.DefaultServerUrlFor(rc)
	if err != nil {
		return err
	}
	u.Path = p

	c.ClientConfig = &client.Config{
		Transport: tc,
		URL:       u,
		Timeout:   c.RESTConfig.Timeout,
	}

	return nil
}

func (c *config) SetVersion() {
	c.Extension.TypeMeta.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   Group,
		Version: Version,
		Kind:    Kind,
	})
}

// GetObject returns the runtime object representation of the configuration.
func (c *config) GetObject() runtime.Object {
	return c.Extension
}

// Get retrieves the current common configuration.
func (c *config) Get() *client.Config {
	return c.ClientConfig
}

func (c *config) Persist(p common.Params) error {
	// Get the config context info for storing configuration
	configContextName, clusterName, userName, err := c.getConfigContextInfo(p)
	if err != nil {
		return err
	}

	// Look for existing config context or create it
	configContext, exists := c.APIConfig.Contexts[configContextName]
	if !exists {
		configContext = &api.Context{
			Cluster:    clusterName,
			AuthInfo:   userName,
			Namespace:  "default",
			Extensions: make(map[string]runtime.Object), // Initialize extensions
		}
		c.APIConfig.Contexts[configContextName] = configContext
	}

	// Ensure Extensions map is initialized even for existing contexts
	if configContext.Extensions == nil {
		configContext.Extensions = make(map[string]runtime.Object)
	}

	// Store/update extension in the config context
	extensionData, err := json.Marshal(c.Extension)
	if err != nil {
		return fmt.Errorf("failed to marshal extension: %w", err)
	}

	configContext.Extensions[ExtensionName] = &runtime.Unknown{
		TypeMeta: c.Extension.TypeMeta,
		Raw:      extensionData,
	}

	return clientcmd.ModifyConfig(c.ConfigAccess, *c.APIConfig, false)
}

// getConfigContextInfo extracts cluster and user information from the current context and constructs
// the config context name for storing Tekton Results configuration.
//
// Parameters:
//   - p: common.Params containing configuration parameters, including the KubeContext.
//
// Returns:
//   - configContextName: The config context name in format "tekton-results-config/{cluster}/{user}".
//   - clusterName: The cluster name from the current context.
//   - userName: The username from the current context.
//   - error: An error if the current context is not set or missing cluster/user information.
func (c *config) getConfigContextInfo(p common.Params) (configContextName, clusterName, userName string, err error) {
	ctx := c.APIConfig.CurrentContext
	if p.KubeContext() != "" {
		ctx = p.KubeContext()
	}

	// Get the context to extract cluster and user info
	context := c.APIConfig.Contexts[ctx]
	if context == nil {
		return "", "", "", errors.New("current context is not set in kubeconfig")
	}

	return common.BuildConfigContextInfo(context)
}

// Set configures the Extension settings for the config object.
// It either prompts the user for input or uses provided parameters to set the values.
//
// Parameters:
//   - prompt: A boolean flag indicating whether to prompt the user for input.
//   - p: A common.Params object containing configuration parameters.
//
// Returns:
//   - error: An error if any step in the configuration process fails, nil otherwise.
func (c *config) Set(prompt bool, p common.Params) error {
	// get data from prompt in enabled
	if prompt {
		host := c.Host()
		if err := c.Prompt("Host", &c.Extension.Host, host); err != nil {
			return err
		}

		token := c.Token()
		if err, ok := token.(error); ok {
			return fmt.Errorf("failed to get token: %w", err)
		}
		if err := c.Prompt("Token", &c.Extension.Token, token); err != nil {
			return err
		}

		if err := c.Prompt("API Path", &c.Extension.APIPath, ""); err != nil {
			return err
		}
		if err := c.Prompt("Insecure Skip TLS Verify", &c.Extension.InsecureSkipTLSVerify, []string{"false", "true"}); err != nil {
			return err
		}
	} else {
		if p.Host() != "" {
			c.Extension.Host = p.Host()
		}
		if p.Token() != "" {
			c.Extension.Token = p.Token()
		}
		if p.APIPath() != "" {
			c.Extension.APIPath = p.APIPath()
		}
		if p.SkipTLSVerify() {
			c.Extension.InsecureSkipTLSVerify = strconv.FormatBool(p.SkipTLSVerify())
		}
	}

	return c.Persist(p)
}

// Reset resets the Tekton Results extension configuration to its default state.//+
//
// Parameters:
//   - p: A common.Params object containing configuration parameters.
//
// Returns an error if the reset process fails, nil otherwise.
func (c *config) Reset(p common.Params) error {
	c.Extension = new(Extension)
	c.SetVersion()
	return c.Persist(p)
}

func (c *config) Prompt(name string, value *string, data any) error {
	var p survey.Prompt

	m := name + " : "

	switch d := data.(type) {
	case string:
		p = &survey.Input{
			Message: m,
			Default: d,
		}
	case []string:
		p = &survey.Select{
			Message: m,
			Options: d,
		}
	default:
		p = &survey.Input{
			Message: m,
		}
	}

	return survey.AskOne(p, value)
}

// LoadExtension loads the Tekton Results extension configuration from the kubeconfig.
// It loads from a dedicated "tekton-results-config" context to ensure configuration
// persists regardless of current namespace context changes (e.g., 'oc project').
//
// Parameters:
//   - p: common.Params containing configuration parameters, including the KubeContext.
//
// Returns:
//   - error: An error if the current context is not set or if there's an issue unmarshaling the extension data.
func (c *config) LoadExtension(p common.Params) error {
	// Get the config context info for loading configuration
	configContextName, _, _, err := c.getConfigContextInfo(p)
	if err != nil {
		return err
	}

	// Check if config context exists
	if configContext, exists := c.APIConfig.Contexts[configContextName]; exists {
		if configContext.Extensions != nil {
			if ext := configContext.Extensions[ExtensionName]; ext != nil {
				// Load existing extension
				c.Extension = new(Extension)
				if err := json.Unmarshal(ext.(*runtime.Unknown).Raw, c.Extension); err != nil {
					return fmt.Errorf("failed to unmarshal extension: %w", err)
				}
				return nil
			}
		}
	}

	// No config context or no extension found - create empty extension
	c.Extension = new(Extension)
	c.SetVersion()
	return nil
}

// Host retrieves the host URL for the Tekton Results API based on external access detection.
// It automatically detects the platform and tries to find a healthy tekton-results-api-service.
//
// Returns:
//   - string: The detected host URL if successful, or empty string if detection fails.
func (c *config) Host() string {
	url, err := getHostURL(c.RESTConfig)
	if err != nil {
		return "" // Return empty string so user can enter manually
	}
	return url // Return the detected URL as default
}

// Token returns the bearer token from the REST configuration.
// It returns an error if the REST configuration is not properly initialized.
//
// Returns:
//   - any: The bearer token string if successful, or an error if the configuration is invalid.
func (c *config) Token() any {
	if c.RESTConfig == nil {
		return fmt.Errorf("REST configuration is not initialized")
	}
	return c.RESTConfig.BearerToken
}

// getRawKubeConfigLoader creates and returns a clientcmd.ClientConfig based on the provided kubeconfig path.
// This function is equivalent to ToRawKubeConfigLoader() and is used to load the kubeconfig file.
//
// Parameters:
//   - kubeconfigPath: A string representing the path to the kubeconfig file.
//
// Returns:
//   - clientcmd.ClientConfig: A non-interactive deferred loading client configuration
//     that uses the specified kubeconfig path and default overrides.
func getRawKubeConfigLoader(kubeconfigPath string) clientcmd.ClientConfig {
	// Set explicit path for kubeconfig
	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	configOverrides := &clientcmd.ConfigOverrides{}

	// Return the clientcmd.ClientConfig (equivalent to ToRawKubeConfigLoader)
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
}

// ServerConnectionFlagsChanged returns true if any server connection flags are set.
func ServerConnectionFlagsChanged(cmd *cobra.Command) bool {
	return cmd.Flags().Changed("host") ||
		cmd.Flags().Changed("token") ||
		cmd.Flags().Changed("insecure-skip-tls-verify") ||
		cmd.Flags().Changed("api-path")
}

// BuildDirectClientConfig builds a client.Config from CLI flags (host, token, api-path, insecure-skip-tls-verify).
func BuildDirectClientConfig(p common.Params) (*client.Config, error) {
	host := p.Host()
	token := p.Token()
	if host == "" || token == "" {
		return nil, errors.New("--host and --token flag must be set if using direct connection flags")
	}
	rc := &rest.Config{
		Host:        host,
		BearerToken: token,
	}
	if p.APIPath() != "" {
		rc.APIPath = p.APIPath()
	}

	rc.Insecure = p.SkipTLSVerify()
	// Optionally set timeout (default 60s)
	rc.Timeout = 60 * time.Second

	rc.APIPath = path.Join(rc.APIPath, Path)

	rc.GroupVersion = &schema.GroupVersion{
		Group:   Group,
		Version: Version,
	}
	u, pth, err := rest.DefaultServerUrlFor(rc)
	if err != nil {
		return nil, err
	}
	u.Path = pth

	tcfg, err := rc.TransportConfig()
	if err != nil {
		return nil, err
	}

	return &client.Config{
		URL:       u,
		Timeout:   rc.Timeout,
		Transport: tcfg,
	}, nil
}

// Validate validates the configuration of the client.
// It checks if the client configuration and extension are properly set up.
//
// Parameters:
//   - c: A Config interface containing the client configuration and extension.
//
// Returns:
//   - error: An error if the configuration is invalid, nil otherwise.
func (c *config) Validate() error {
	// Check if the configuration is properly set up
	clientConfig := c.Get()
	if clientConfig == nil || clientConfig.URL == nil {
		return fmt.Errorf("client configuration missing: URL not set")
	}

	// Check if essential configuration values are missing
	extensionObj := c.GetObject()
	extension, ok := extensionObj.(*Extension)
	if !ok {
		return fmt.Errorf("invalid extension type: expected *Extension, got %T", extensionObj)
	}

	if extension.Host == "" {
		return fmt.Errorf("API server host not configured: host field is empty")
	}

	return nil
}
