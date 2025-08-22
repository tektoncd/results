package common

import (
	"errors"
	"fmt"
	"strings"

	"k8s.io/client-go/tools/clientcmd/api"
)

// BuildConfigContextInfo extracts cluster name, username, and constructs the config context name
// from a Kubernetes context.
//
// Parameters:
//   - context: The Kubernetes context object from kubeconfig
//
// Returns:
//   - configContextName: The config context name in format "tekton-results-config/{cluster}/{user}".
//   - clusterName: The cluster name from the context.
//   - userName: The extracted username (part before "/" if present).
//   - error: An error if the context is missing cluster/user information.
func BuildConfigContextInfo(context *api.Context) (configContextName, clusterName, userName string, err error) {
	if context == nil {
		return "", "", "", errors.New("context is nil")
	}

	clusterName = context.Cluster
	if clusterName == "" {
		return "", "", "", errors.New("no cluster specified in context")
	}

	userName = context.AuthInfo
	if userName == "" {
		return "", "", "", errors.New("no user specified in context")
	}

	// Extract just the username part before "/" for config context isolation
	// In some cases user also has cluster name in the format "user/cluster"
	if slashIndex := strings.Index(userName, "/"); slashIndex != -1 {
		userName = userName[:slashIndex]
	}

	// Construct the config context name (tekton-results-config context for config storage)
	configContextName = fmt.Sprintf("tekton-results-config/%s/%s", clusterName, userName)
	return configContextName, clusterName, userName, nil
}
