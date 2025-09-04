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

// Configuration methods
func (p *Params) SetKubeConfigPath(path string) { p.kubeConfigPath = path }
func (p *Params) KubeConfigPath() string        { return p.kubeConfigPath }
func (p *Params) SetKubeContext(context string) { p.kubeContext = context }
func (p *Params) KubeContext() string           { return p.kubeContext }
func (p *Params) SetNamespace(ns string) {
	// For testing, simulate the kubeconfig resolution behavior:
	// If empty string is provided, keep the existing namespace (simulates kubeconfig default)
	if ns != "" {
		p.namespace = ns
	}
	// If ns is empty, keep the existing namespace (set in NewParams() as "default")
}
func (p *Params) Namespace() string          { return p.namespace }
func (p *Params) SetHost(host string)        { p.host = host }
func (p *Params) Host() string               { return p.host }
func (p *Params) SetToken(token string)      { p.token = token }
func (p *Params) Token() string              { return p.token }
func (p *Params) SetAPIPath(path string)     { p.apiPath = path }
func (p *Params) APIPath() string            { return p.apiPath }
func (p *Params) SetSkipTLSVerify(skip bool) { p.skipTLSVerify = skip }
func (p *Params) SkipTLSVerify() bool        { return p.skipTLSVerify }

// Client injection methods for testing
func (p *Params) SetRESTClient(client *client.RESTClient) {
	p.restClient = client
}

// RESTClient returns the injected REST client for testing
func (p *Params) RESTClient() *client.RESTClient {
	return p.restClient
}
