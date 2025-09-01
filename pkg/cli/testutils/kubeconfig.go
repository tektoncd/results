// Package testutils provides test utility functions for the CLI package
package testutils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/tektoncd/results/pkg/cli/common"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
)

// CreateTestKubeconfig creates a temporary kubeconfig file for testing
// If namespace is empty, no namespace is set in the context
func CreateTestKubeconfig(t *testing.T, namespace string) string {
	t.Helper()

	dir := t.TempDir()
	kubeconfigPath := filepath.Join(dir, "kubeconfig.yaml")

	// Build context section with optional namespace
	contextSection := `    cluster: test-cluster
    user: test-user`
	if namespace != "" {
		contextSection = fmt.Sprintf(`    cluster: test-cluster
    namespace: %s
    user: test-user`, namespace)
	}

	kubeconfigContent := fmt.Sprintf(`apiVersion: v1
clusters:
- cluster:
    server: http://test-host
  name: test-cluster
contexts:
- context:
%s
  name: test-context
current-context: test-context
kind: Config
preferences: {}
users:
- name: test-user
`, contextSection)

	if err := os.WriteFile(kubeconfigPath, []byte(kubeconfigContent), 0600); err != nil {
		t.Fatalf("Failed to write kubeconfig: %v", err)
	}

	return kubeconfigPath
}

// ReadKubeconfigExtensionRaw reads the tekton-results extension from a kubeconfig file as raw data
// This avoids import cycles by not depending on config package types
func ReadKubeconfigExtensionRaw(t *testing.T, kubeconfigPath, extensionName string) ([]byte, error) {
	t.Helper()

	configLoadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	apiConfig, err := configLoadingRules.Load()
	if err != nil {
		return nil, err
	}

	context := apiConfig.Contexts[apiConfig.CurrentContext]
	if context == nil {
		return nil, nil
	}

	// Use the shared function from common package to build context info
	configContextName, _, _, err := common.BuildConfigContextInfo(context)
	if err != nil {
		return nil, err
	}

	// Look for config context using direct lookup
	configContext, exists := apiConfig.Contexts[configContextName]
	if !exists || configContext.Extensions == nil {
		return nil, nil
	}

	extensionData, exists := configContext.Extensions[extensionName]
	if !exists {
		return nil, nil
	}

	// Return the raw extension data
	return extensionData.(*runtime.Unknown).Raw, nil
}

// ReadKubeconfigExtension reads and unmarshals a kubeconfig extension into the provided target
// The target parameter should be a pointer to the struct you want to unmarshal into
// Returns true if the extension was found and unmarshaled successfully, false if not found
func ReadKubeconfigExtension(t *testing.T, kubeconfigPath, extensionName string, target interface{}) (bool, error) {
	t.Helper()

	rawData, err := ReadKubeconfigExtensionRaw(t, kubeconfigPath, extensionName)
	if err != nil {
		return false, err
	}
	if rawData == nil {
		return false, nil
	}

	// Unmarshal the raw data into the target
	if err := json.Unmarshal(rawData, target); err != nil {
		return false, err
	}

	return true, nil
}

// UpdateKubeconfigNamespace updates the namespace of the current context in a kubeconfig file
func UpdateKubeconfigNamespace(t *testing.T, kubeconfigPath, newNamespace string) {
	t.Helper()

	// Load the kubeconfig
	configLoadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	apiConfig, err := configLoadingRules.Load()
	if err != nil {
		t.Fatalf("Failed to load kubeconfig: %v", err)
	}

	// Update the namespace of the current context
	currentContext := apiConfig.Contexts[apiConfig.CurrentContext]
	if currentContext == nil {
		t.Fatalf("Current context not found in kubeconfig")
	}

	currentContext.Namespace = newNamespace

	// Write the updated kubeconfig
	if err := clientcmd.WriteToFile(*apiConfig, kubeconfigPath); err != nil {
		t.Fatalf("Failed to write updated kubeconfig: %v", err)
	}
}
