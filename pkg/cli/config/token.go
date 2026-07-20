package config

import (
	"errors"
	"net/http"
	"strings"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/transport"
)

// resolveBearerToken returns the bearer token that client-go would attach to a
// request built from the given rest.Config.
//
// A kubeconfig context does not always carry a static bearer token: the token
// may instead be produced at request time by an exec credential plugin (e.g.
// `oc get-token` for OpenShift external OIDC, `aws eks get-token`, etc.) or a
// legacy auth-provider (oidc/gcp/azure). In those cases rest.Config.BearerToken
// is empty and the credential is injected via the transport's WrapTransport
// round-tripper chain instead.
//
// This helper drives that round-tripper chain once so the exec plugin /
// auth-provider is actually invoked, and captures the resulting
// "Authorization: Bearer <token>" header — the same token oc/kubectl/tkn would
// send. If the config already has a static token, that is returned directly and
// no plugin is run.
//
// It returns an empty string (and no error) when the context authenticates by
// some means other than a bearer token (e.g. client certificate), since there
// is no token to resolve in that case.
func resolveBearerToken(rc *rest.Config) (string, error) {
	if rc == nil {
		return "", errors.New("nil REST config provided")
	}

	// Fast path: a static token is already present.
	if rc.BearerToken != "" {
		return rc.BearerToken, nil
	}

	tc, err := rc.TransportConfig()
	if err != nil {
		return "", err
	}

	// If the transport does not wrap requests with a credential provider
	// (exec/auth-provider), there is no bearer token to resolve here.
	if tc.WrapTransport == nil {
		return "", nil
	}

	// Build the credential-aware round-tripper chain and run a single request
	// through it. The terminal round-tripper never touches the network: it just
	// captures whatever Authorization header the chain set.
	capture := &authHeaderCapturingRoundTripper{}
	rt, err := transport.HTTPWrappersForConfig(tc, capture)
	if err != nil {
		return "", err
	}

	// A minimal request; the host is irrelevant because capture short-circuits
	// before any network I/O.
	req, err := http.NewRequest(http.MethodGet, "https://tekton-results.local/", nil)
	if err != nil {
		return "", err
	}
	if _, err := rt.RoundTrip(req); err != nil {
		return "", err
	}

	authz := capture.authorization
	if authz == "" {
		return "", nil
	}
	// Strip the "Bearer " scheme prefix if present.
	if parts := strings.SplitN(authz, " ", 2); len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1], nil
	}
	return authz, nil
}

// authHeaderCapturingRoundTripper is a terminal http.RoundTripper that records
// the Authorization header set by the wrapping credential round-trippers and
// returns a stub response without performing any network I/O.
type authHeaderCapturingRoundTripper struct {
	authorization string
}

func (r *authHeaderCapturingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r.authorization = req.Header.Get("Authorization")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       http.NoBody,
		Request:    req,
	}, nil
}
