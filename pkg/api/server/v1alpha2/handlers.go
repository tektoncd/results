// Package server provides the v1alpha2 API server implementation.
package server

import (
	"context"
	"net/http"

	"github.com/tektoncd/results/pkg/api/server/v1alpha2/plugin"
)

// Handler returns a http.Handler that serves the gRPC server and the log plugin server
func Handler(grpcMux http.Handler, pluginServer *plugin.LogServer) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), directRESTRequestKey{}, &directRESTRequest{})
		grpcMux.ServeHTTP(w, r.WithContext(ctx))
	}))
	if pluginServer != nil && pluginServer.IsLogPluginEnabled {
		mux.Handle("/apis/results.tekton.dev/v1alpha2/parents/{parent}/results/{resultID}/logs/{recordID}", pluginServer.LogMux())
		mux.Handle("/apis/results.tekton.dev/v1alpha3/parents/{parent}/results/{resultID}/logs/{recordID}", pluginServer.LogMux())
	}
	return mux
}
