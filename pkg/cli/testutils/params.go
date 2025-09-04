package testutils

import (
	"github.com/tektoncd/results/pkg/cli/client"
	"github.com/tektoncd/results/pkg/cli/common"
)

// Params implements common.Params interface for testing
// This allows injecting fake/mock clients during tests
type Params struct {
	kubeConfigPath string
	kubeContext    string
	namespace      string
	host           string
	token          string
	apiPath        string
	skipTLSVerify  bool

	// Simple client storage for testing
	restClient *client.RESTClient
}

// Ensure Params implements the interface
var _ common.Params = (*Params)(nil)

// NewParams creates a new test Params with sensible defaults
func NewParams() *Params {
	return &Params{
		host:      "http://localhost:8080",
		namespace: "default",
	}
}

// SetKubeConfigPath sets the kubeconfig file path
func (p *Params) SetKubeConfigPath(path string) { p.kubeConfigPath = path }

// KubeConfigPath returns the kubeconfig file path
func (p *Params) KubeConfigPath() string { return p.kubeConfigPath }

// SetKubeContext sets the kubernetes context
func (p *Params) SetKubeContext(context string) { p.kubeContext = context }

// KubeContext returns the kubernetes context
func (p *Params) KubeContext() string { return p.kubeContext }

// SetNamespace sets the kubernetes namespace, preserving default if empty
func (p *Params) SetNamespace(ns string) {
	// For testing, simulate the kubeconfig resolution behavior:
	// If empty string is provided, keep the existing namespace (simulates kubeconfig default)
	if ns != "" {
		p.namespace = ns
	}
	// If ns is empty, keep the existing namespace (set in NewParams() as "default")
}

// Namespace returns the kubernetes namespace
func (p *Params) Namespace() string { return p.namespace }

// SetHost sets the API host
func (p *Params) SetHost(host string) { p.host = host }

// Host returns the API host
func (p *Params) Host() string { return p.host }

// SetToken sets the authentication token
func (p *Params) SetToken(token string) { p.token = token }

// Token returns the authentication token
func (p *Params) Token() string { return p.token }

// SetAPIPath sets the API path
func (p *Params) SetAPIPath(path string) { p.apiPath = path }

// APIPath returns the API path
func (p *Params) APIPath() string { return p.apiPath }

// SetSkipTLSVerify sets whether to skip TLS verification
func (p *Params) SetSkipTLSVerify(skip bool) { p.skipTLSVerify = skip }

// SkipTLSVerify returns whether to skip TLS verification
func (p *Params) SkipTLSVerify() bool { return p.skipTLSVerify }

// SetRESTClient injects a REST client for testing purposes
func (p *Params) SetRESTClient(client *client.RESTClient) {
	p.restClient = client
}

// RESTClient returns the injected REST client for testing
func (p *Params) RESTClient() *client.RESTClient {
	return p.restClient
}
