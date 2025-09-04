package common

import (
	"io"

	"github.com/tektoncd/results/pkg/cli/client"
)

// Stream for input and output
type Stream struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// Params interface provides
type Params interface {
	// SetKubeConfigPath uses the kubeconfig path to instantiate tekton
	// returned by Clientset function
	SetKubeConfigPath(string)
	KubeConfigPath() string

	// SetKubeContext extends the specificity of the above SetKubeConfigPath
	// by using a context other than the default context in the given kubeconfig
	SetKubeContext(string)
	KubeContext() string

	// SetNamespace can be used to store the namespace parameter that is needed
	// by most commands
	SetNamespace(string)
	Namespace() string

	// SetHost can be used to store the host parameter that is needed
	// by most commands
	SetHost(string)
	Host() string

	// SetToken can be used to store the token parameter that is needed
	// by most commands
	SetToken(string)
	Token() string

	// SetApiPath can be used to store the api-path parameter that is needed
	// by most commands
	SetAPIPath(string)
	APIPath() string

	// SetSkipTlsVerify can be used to store the api-path parameter that is needed
	// by most commands
	SetSkipTLSVerify(bool)
	SkipTLSVerify() bool

	// Client access method for dependency injection
	// Returns REST client - from which log/record clients are created
	SetRESTClient(client *client.RESTClient)
	RESTClient() *client.RESTClient
}
