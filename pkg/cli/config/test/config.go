package test

import (
	"github.com/tektoncd/results/pkg/cli/client"
	"github.com/tektoncd/results/pkg/cli/common"
	"github.com/tektoncd/results/pkg/cli/config"
	"k8s.io/apimachinery/pkg/runtime"
)

// Params implements common.Params interface for testing
type Params struct {
	kubeConfigPath string
	kubeContext    string
	namespace      string
	host           string
	token          string
	apiPath        string
	skipTLSVerify  bool
}

// APIPath implements the APIPath method for testing
func (p *Params) APIPath() string {
	return p.apiPath
}

// SetAPIPath implements the SetAPIPath method for testing
func (p *Params) SetAPIPath(path string) {
	p.apiPath = path
}

// Host implements the Host method for testing
func (p *Params) Host() string {
	return p.host
}

// SetHost implements the SetHost method for testing
func (p *Params) SetHost(host string) {
	p.host = host
}

// KubeConfigPath implements the KubeConfigPath method for testing
func (p *Params) KubeConfigPath() string {
	return p.kubeConfigPath
}

// SetKubeConfigPath implements the SetKubeConfigPath method for testing
func (p *Params) SetKubeConfigPath(path string) {
	p.kubeConfigPath = path
}

// KubeContext implements the KubeContext method for testing
func (p *Params) KubeContext() string {
	return p.kubeContext
}

// SetKubeContext implements the SetKubeContext method for testing
func (p *Params) SetKubeContext(context string) {
	p.kubeContext = context
}

// Namespace implements the Namespace method for testing
func (p *Params) Namespace() string {
	return p.namespace
}

// SetNamespace implements the SetNamespace method for testing
func (p *Params) SetNamespace(ns string) {
	p.namespace = ns
}

// Token implements the Token method for testing
func (p *Params) Token() string {
	return p.token
}

// SetToken implements the SetToken method for testing
func (p *Params) SetToken(token string) {
	p.token = token
}

// SkipTLSVerify implements the SkipTLSVerify method for testing
func (p *Params) SkipTLSVerify() bool {
	return p.skipTLSVerify
}

// SetSkipTLSVerify implements the SetSkipTLSVerify method for testing
func (p *Params) SetSkipTLSVerify(skip bool) {
	p.skipTLSVerify = skip
}

// Config implements config.Config interface for testing
type Config struct {
	SetError error
}

// NewConfig creates a new test config
func (c *Config) NewConfig(_ common.Params) (config.Config, error) {
	return c, nil
}

// Set implements the Set method for testing
func (c *Config) Set(_ bool, _ common.Params) error {
	// Always return the SetError to ensure it's propagated
	return c.SetError
}

// Get implements the Get method for testing
func (c *Config) Get() *client.Config {
	return &client.Config{}
}

// GetObject implements the GetObject method for testing
func (c *Config) GetObject() runtime.Object {
	return nil
}

// Reset implements the Reset method for testing
func (c *Config) Reset(_ common.Params) error {
	return nil
}
