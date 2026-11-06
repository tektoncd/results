package main

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/tektoncd/results/pkg/api/server/config"
)

func TestGatewayImpersonationParity(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, tc := range []struct {
			name    string
			headers http.Header
			want    int
		}{
			{"all attributes", http.Header{"Impersonate-User": {"impersonated"}, "Impersonate-Group": {"developers", "reviewers"}, "Impersonate-Uid": {"impersonated-uid"}, "Impersonate-Extra-Scope": {"build", "read"}}, 200},
			{"denied user", http.Header{"Impersonate-User": {"denied"}}, 401},
			{"group without user", http.Header{"Impersonate-Group": {"developers"}}, 401},
			{"no impersonation", http.Header{}, 200},
		} {
			t.Run(tc.name, func(t *testing.T) {
				var oldSpecs []string
				var oldBody map[string]any
				want := tc.want
				if !enabled {
					want = 200
				}
				for _, baseline := range []bool{true, false} {
					client, checker := gatewayRBAC(enabled)
					g := newGatewayFixture(t, baseline, &config.Config{AUTH_IMPERSONATE: enabled}, checker)
					headers := tc.headers.Clone()
					headers.Set("Authorization", "Bearer authorized")
					body := gatewayJSON(t, gatewayRequest(t, g.handler, "GET", resultsURL, nil, headers), want)
					specs := reviewSpecs(t, client)
					if baseline {
						oldSpecs, oldBody = specs, body
					} else if !reflect.DeepEqual(oldSpecs, specs) || !reflect.DeepEqual(oldBody, body) {
						t.Errorf("impersonation parity enabled=%v: baseline=%v direct=%v", enabled, oldSpecs, specs)
					}
				}
			})
		}
	}
}
