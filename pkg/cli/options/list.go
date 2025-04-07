package options

import "github.com/tektoncd/results/pkg/cli/client"

// ListOptions holds the options for listing resources
type ListOptions struct {
	Client        *client.RESTClient
	Limit         int32
	AllNamespaces bool
	Label         string
	SinglePage    bool
	ResourceName  string
}
