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
// may instead be read from a token file or produced at request time by an exec
// credential plugin (e.g. `oc get-token` for OpenShift external OIDC,
// `aws eks get-token`, etc.) or a legacy auth-provider (oidc/gcp/azure). In
// those cases rest.Config.BearerToken is empty and the credential is injected by
// client-go's transport wrappers instead.
//
// This helper drives those round-tripper wrappers once so token files, exec
// plugins, and auth-providers are resolved, and captures the resulting
// "Authorization: Bearer <token>" header - the same token oc/kubectl/tkn would
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

	// Build the credential-aware round-tripper chain and run a single request
	// through it. The terminal round-tripper never touches the network: it just
	// returns the request after wrapper-injected credentials have been applied.
	rt, err := transport.HTTPWrappersForConfig(tc, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       http.NoBody,
			Request:    req,
		}, nil
	}))
	if err != nil {
		return "", err
	}

	// A minimal request; the host is irrelevant because the chain short-circuits
	// before any network I/O.
	req, err := http.NewRequest(http.MethodGet, "https://tekton-results.local/", nil)
	if err != nil {
		return "", err
	}
	resp, err := rt.RoundTrip(req)
	if err != nil {
		return "", err
	}

	authz := resp.Request.Header.Get("Authorization")
	if authz == "" {
		return "", nil
	}
	// Return only bearer tokens; other Authorization schemes are not usable as
	// Results API tokens.
	if parts := strings.SplitN(authz, " ", 2); len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1], nil
	}
	return "", nil
}

// roundTripperFunc adapts a function to http.RoundTripper.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
