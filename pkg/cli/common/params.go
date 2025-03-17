package common

// ResultsParams holds configuration parameters for interacting with Kubernetes and API endpoints.
type ResultsParams struct {
	kubeConfigPath string
	kubeContext    string
	namespace      string
	host           string
	token          string
	apiPath        string
	skipTlsVerify  bool
}

var _ Params = (*ResultsParams)(nil)

// KubeConfigPath returns the path to the Kubernetes configuration file.
func (p *ResultsParams) KubeConfigPath() string {
	return p.kubeConfigPath
}

// KubeContext returns the Kubernetes context name.
func (p *ResultsParams) KubeContext() string {
	return p.kubeContext
}

// SetKubeConfigPath sets the path to the Kubernetes configuration file.
//
// Parameters:
//   - path: The file path to the Kubernetes configuration.
func (p *ResultsParams) SetKubeConfigPath(path string) {
	p.kubeConfigPath = path
}

// SetKubeContext sets the Kubernetes context name.
//
// Parameters:
//   - context: The name of the Kubernetes context to use.
func (p *ResultsParams) SetKubeContext(context string) {
	p.kubeContext = context
}

// SetNamespace sets the Kubernetes namespace.
//
// Parameters:
//   - ns: The namespace to set.
func (p *ResultsParams) SetNamespace(ns string) {
	p.namespace = ns
}

// Namespace returns the current Kubernetes namespace.
func (p *ResultsParams) Namespace() string {
	return p.namespace
}

// Host returns the API server host address.
func (p *ResultsParams) Host() string {
	return p.host
}

// SetHost sets the API server host address.
//
// Parameters:
//   - host: The host address to set.
func (p *ResultsParams) SetHost(host string) {
	p.host = host
}

// Token returns the authentication token for API requests.
func (p *ResultsParams) Token() string {
	return p.token
}

// SetToken sets the authentication token for API requests.
//
// Parameters:
//   - token: The authentication token to set.
func (p *ResultsParams) SetToken(token string) {
	p.token = token
}

// ApiPath returns the API endpoint path.
func (p *ResultsParams) ApiPath() string {
	return p.apiPath
}

// SetApiPath sets the API endpoint path.
//
// Parameters:
//   - apiPath: The API endpoint path to set.
func (p *ResultsParams) SetApiPath(apiPath string) {
	p.apiPath = apiPath
}

// SkipTlsVerify returns whether TLS certificate verification should be skipped.
func (p *ResultsParams) SkipTlsVerify() bool {
	return p.skipTlsVerify
}

// SetSkipTlsVerify sets whether TLS certificate verification should be skipped.
//
// Parameters:
//   - skipTlsVerify: Boolean indicating whether to skip TLS verification.
func (p *ResultsParams) SetSkipTlsVerify(skipTlsVerify bool) {
	p.skipTlsVerify = skipTlsVerify
}
