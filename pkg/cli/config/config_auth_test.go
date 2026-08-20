package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/tektoncd/results/pkg/cli/common"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// writeKubeconfig writes the given api.Config to a temp file and returns its path.
func writeKubeconfig(t *testing.T, cfg *clientcmdapi.Config) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig.yaml")
	if err := clientcmd.WriteToFile(*cfg, path); err != nil {
		t.Fatalf("failed to write kubeconfig: %v", err)
	}
	return path
}

// restConfigFromKubeconfig loads a *rest.Config from a kubeconfig file using the
// same non-interactive deferred loader NewConfig uses.
func restConfigFromKubeconfig(t *testing.T, path string) *rest.Config {
	t.Helper()
	loader := getRawKubeConfigLoader(path)
	rc, err := loader.ClientConfig()
	if err != nil {
		t.Fatalf("failed to build rest.Config from %s: %v", path, err)
	}
	return rc
}

// staticTokenKubeconfig returns an api.Config whose only user authenticates with
// a static bearer token.
func staticTokenKubeconfig(token string) *clientcmdapi.Config {
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["test-cluster"] = &clientcmdapi.Cluster{
		Server: "https://test-host:6443",
	}
	cfg.AuthInfos["test-user"] = &clientcmdapi.AuthInfo{
		Token: token,
	}
	cfg.Contexts["test-context"] = &clientcmdapi.Context{
		Cluster:  "test-cluster",
		AuthInfo: "test-user",
	}
	cfg.CurrentContext = "test-context"
	return cfg
}

// execTokenKubeconfig returns an api.Config whose current-context user
// authenticates via an exec credential plugin. The plugin is a small shell
// script (written to a temp file) that prints an ExecCredential with the given
// token. This mirrors real-world exec credentials such as `oc get-token`,
// `aws eks get-token`, or `gke-gcloud-auth-plugin`, without any network access.
func execTokenKubeconfig(t *testing.T, token string) *clientcmdapi.Config {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("exec credential plugin test uses a POSIX shell script")
	}

	dir := t.TempDir()
	plugin := filepath.Join(dir, "fake-exec-plugin.sh")
	script := fmt.Sprintf(`#!/bin/sh
cat <<'JSON'
{
  "apiVersion": "client.authentication.k8s.io/v1",
  "kind": "ExecCredential",
  "status": {
    "token": %q
  }
}
JSON
`, token)
	if err := os.WriteFile(plugin, []byte(script), 0o700); err != nil {
		t.Fatalf("failed to write exec plugin: %v", err)
	}

	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["test-cluster"] = &clientcmdapi.Cluster{
		Server: "https://test-host:6443",
	}
	cfg.AuthInfos["test-user"] = &clientcmdapi.AuthInfo{
		Exec: &clientcmdapi.ExecConfig{
			APIVersion:      "client.authentication.k8s.io/v1",
			Command:         plugin,
			InteractiveMode: clientcmdapi.NeverExecInteractiveMode,
		},
	}
	cfg.Contexts["test-context"] = &clientcmdapi.Context{
		Cluster:  "test-cluster",
		AuthInfo: "test-user",
	}
	cfg.CurrentContext = "test-context"
	return cfg
}

// TestGetRawKubeConfigLoaderHonorsKUBECONFIG verifies that the kubeconfig loader
// honors the $KUBECONFIG environment variable (previously it hardcoded
// ~/.kube/config and only respected an explicit --kubeconfig path).
func TestGetRawKubeConfigLoaderHonorsKUBECONFIG(t *testing.T) {
	// A context that only exists in a file referenced via $KUBECONFIG.
	kubeconfigPath := writeKubeconfig(t, staticTokenKubeconfig("kubeconfig-env-token"))

	// Ensure the default home file is NOT used by pointing HOME at an empty dir.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KUBECONFIG", kubeconfigPath)

	// explicitPath empty => should fall back to $KUBECONFIG.
	loader := getRawKubeConfigLoader("")
	rawConfig, err := loader.RawConfig()
	if err != nil {
		t.Fatalf("RawConfig() failed: %v", err)
	}
	if rawConfig.CurrentContext != "test-context" {
		t.Fatalf("expected current-context from $KUBECONFIG file, got %q", rawConfig.CurrentContext)
	}
	if _, ok := rawConfig.Contexts["test-context"]; !ok {
		t.Fatalf("expected context from $KUBECONFIG to be loaded; contexts: %v", rawConfig.Contexts)
	}
}

// TestGetRawKubeConfigLoaderExplicitPathTakesPrecedence verifies that an
// explicit --kubeconfig path wins over $KUBECONFIG.
func TestGetRawKubeConfigLoaderExplicitPathTakesPrecedence(t *testing.T) {
	envPath := writeKubeconfig(t, func() *clientcmdapi.Config {
		c := staticTokenKubeconfig("env-token")
		// Rename the context so we can tell the files apart.
		c.Contexts["env-context"] = c.Contexts["test-context"]
		delete(c.Contexts, "test-context")
		c.CurrentContext = "env-context"
		return c
	}())
	explicitPath := writeKubeconfig(t, func() *clientcmdapi.Config {
		c := staticTokenKubeconfig("explicit-token")
		c.Contexts["explicit-context"] = c.Contexts["test-context"]
		delete(c.Contexts, "test-context")
		c.CurrentContext = "explicit-context"
		return c
	}())

	t.Setenv("HOME", t.TempDir())
	t.Setenv("KUBECONFIG", envPath)

	loader := getRawKubeConfigLoader(explicitPath)
	rawConfig, err := loader.RawConfig()
	if err != nil {
		t.Fatalf("RawConfig() failed: %v", err)
	}
	if rawConfig.CurrentContext != "explicit-context" {
		t.Fatalf("expected explicit --kubeconfig to take precedence, got current-context %q", rawConfig.CurrentContext)
	}
}

// TestNewConfigHonorsKUBECONFIG verifies end-to-end that NewConfig resolves the
// current context from $KUBECONFIG when no --kubeconfig flag is provided.
// Previously this failed with "context ” not found in kubeconfig".
func TestNewConfigHonorsKUBECONFIG(t *testing.T) {
	kubeconfigPath := writeKubeconfig(t, staticTokenKubeconfig("kubeconfig-env-token"))

	t.Setenv("HOME", t.TempDir())
	t.Setenv("KUBECONFIG", kubeconfigPath)

	p := &common.ResultsParams{}
	// Note: KubeConfigPath is intentionally NOT set, so resolution must come
	// from $KUBECONFIG.
	cfg, err := NewConfig(p)
	if err != nil {
		t.Fatalf("NewConfig() failed (should honor $KUBECONFIG): %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	token := cfg.(*config).Token()
	if tokenStr, ok := token.(string); !ok || tokenStr != "kubeconfig-env-token" {
		t.Fatalf("expected token resolved from $KUBECONFIG file, got %v (%T)", token, token)
	}
}

// TestTokenExecCredential verifies Token() end-to-end through NewConfig resolves
// an exec-credential token (previously it returned an empty string because it
// read rest.Config.BearerToken directly).
func TestTokenExecCredential(t *testing.T) {
	kubeconfigPath := writeKubeconfig(t, execTokenKubeconfig(t, "exec-minted-token-via-token"))

	p := &common.ResultsParams{}
	p.SetKubeConfigPath(kubeconfigPath)
	p.SetKubeContext("test-context")
	cfg, err := NewConfig(p)
	if err != nil {
		t.Fatalf("NewConfig() failed: %v", err)
	}

	token := cfg.(*config).Token()
	tokenStr, ok := token.(string)
	if !ok {
		t.Fatalf("expected Token() to return a string, got %T (%v)", token, token)
	}
	if tokenStr != "exec-minted-token-via-token" {
		t.Fatalf("expected Token() to resolve exec token, got %q", tokenStr)
	}
}
