package config

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// TestUsesDynamicToken verifies source detection: static tokens are safe to
// persist, while exec plugins, auth-providers, and token files yield
// short-lived credentials that must not be pre-filled/persisted.
func TestUsesDynamicToken(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("failed to write token file: %v", err)
	}

	cases := []struct {
		name string
		rc   *rest.Config
		want bool
	}{
		{"nil config", nil, false},
		{"static token", &rest.Config{BearerToken: "static"}, false},
		{"static token plus token file (static wins)", &rest.Config{BearerToken: "static", BearerTokenFile: tokenFile}, false},
		{"exec provider", &rest.Config{ExecProvider: &clientcmdapi.ExecConfig{Command: "oc"}}, true},
		{"auth provider", &rest.Config{AuthProvider: &clientcmdapi.AuthProviderConfig{Name: "oidc"}}, true},
		{"token file", &rest.Config{BearerTokenFile: tokenFile}, true},
		{"no credential", &rest.Config{Host: "https://test-host:6443"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := usesDynamicToken(tc.rc); got != tc.want {
				t.Fatalf("usesDynamicToken() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestResolveBearerTokenStaticToken verifies the fast path: a static token in
// the config is returned directly without invoking any credential provider.
func TestResolveBearerTokenStaticToken(t *testing.T) {
	rc := &rest.Config{BearerToken: "static-token-abc"}
	got, err := resolveBearerToken(rc)
	if err != nil {
		t.Fatalf("resolveBearerToken() error: %v", err)
	}
	if got != "static-token-abc" {
		t.Fatalf("expected static token to be returned, got %q", got)
	}
}

// TestResolveBearerTokenFile verifies client-go's token-file bearer wrapper is
// exercised even when no custom WrapTransport is present.
func TestResolveBearerTokenFile(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("token-file-abc"), 0o600); err != nil {
		t.Fatalf("failed to write token file: %v", err)
	}

	rc := &rest.Config{Host: "https://test-host:6443", BearerTokenFile: tokenFile}
	got, err := resolveBearerToken(rc)
	if err != nil {
		t.Fatalf("resolveBearerToken() error: %v", err)
	}
	if got != "token-file-abc" {
		t.Fatalf("expected token from token file, got %q", got)
	}
}

// TestResolveBearerTokenNil verifies resolveBearerToken errors on a nil config.
func TestResolveBearerTokenNil(t *testing.T) {
	if _, err := resolveBearerToken(nil); err == nil {
		t.Fatal("expected error for nil rest.Config")
	}
}

// TestResolveBearerTokenNoCredential verifies that a config with neither a
// static token nor a credential provider (e.g. a client-cert-only context)
// resolves to an empty token without error, rather than failing.
func TestResolveBearerTokenNoCredential(t *testing.T) {
	rc := &rest.Config{Host: "https://test-host:6443"}
	got, err := resolveBearerToken(rc)
	if err != nil {
		t.Fatalf("resolveBearerToken() error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty token for non-token auth, got %q", got)
	}
}

// TestResolveBearerTokenExecCredential verifies the core fix: when the current
// context authenticates via an exec credential plugin (no static token), the
// plugin is invoked and its minted token is resolved. This mirrors the
// OpenShift external-OIDC (`oc get-token`) / custom-plugin cases.
func TestResolveBearerTokenExecCredential(t *testing.T) {
	kubeconfigPath := writeKubeconfig(t, execTokenKubeconfig(t, "exec-minted-token-xyz"))

	rc := restConfigFromKubeconfig(t, kubeconfigPath)

	// Sanity: the exec case must have an empty static BearerToken and a
	// populated ExecProvider — otherwise the test isn't exercising the fix.
	if rc.BearerToken != "" {
		t.Fatalf("precondition: expected empty static BearerToken, got %q", rc.BearerToken)
	}
	if rc.ExecProvider == nil {
		t.Fatal("precondition: expected ExecProvider to be set for exec credential")
	}

	got, err := resolveBearerToken(rc)
	if err != nil {
		t.Fatalf("resolveBearerToken() error: %v", err)
	}
	if got != "exec-minted-token-xyz" {
		t.Fatalf("expected exec plugin token to be resolved, got %q", got)
	}
}

// TestResolveBearerTokenWrapTransport verifies that when the credential is
// injected purely through the transport's WrapTransport chain (as exec plugins
// and auth-providers do), resolveBearerToken drives that chain and captures the
// resulting Authorization header. Here the wrapper sets a "Bearer" header, which
// should have its scheme prefix stripped.
func TestResolveBearerTokenWrapTransport(t *testing.T) {
	rc := &rest.Config{Host: "https://test-host:6443"}
	rc.WrapTransport = func(inner http.RoundTripper) http.RoundTripper {
		return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			req.Header.Set("Authorization", "Bearer wrapped-token-123")
			return inner.RoundTrip(req)
		})
	}

	got, err := resolveBearerToken(rc)
	if err != nil {
		t.Fatalf("resolveBearerToken() error: %v", err)
	}
	if got != "wrapped-token-123" {
		t.Fatalf("expected token from WrapTransport chain, got %q", got)
	}
}

// TestResolveBearerTokenNonBearerScheme verifies that a non-"Bearer"
// Authorization header set by the transport chain is not returned as a Results
// API token.
func TestResolveBearerTokenNonBearerScheme(t *testing.T) {
	rc := &rest.Config{Host: "https://test-host:6443"}
	rc.WrapTransport = func(inner http.RoundTripper) http.RoundTripper {
		return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			req.Header.Set("Authorization", "Custom raw-value")
			return inner.RoundTrip(req)
		})
	}

	got, err := resolveBearerToken(rc)
	if err != nil {
		t.Fatalf("resolveBearerToken() error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty token for non-Bearer Authorization value, got %q", got)
	}
}

// TestResolveBearerTokenWrapTransportNoAuth verifies that a WrapTransport chain
// that sets no Authorization header resolves to an empty token without error.
func TestResolveBearerTokenWrapTransportNoAuth(t *testing.T) {
	rc := &rest.Config{Host: "https://test-host:6443"}
	rc.WrapTransport = func(inner http.RoundTripper) http.RoundTripper {
		return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			// Intentionally set no Authorization header.
			return inner.RoundTrip(req)
		})
	}

	got, err := resolveBearerToken(rc)
	if err != nil {
		t.Fatalf("resolveBearerToken() error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty token when no Authorization header is set, got %q", got)
	}
}
