package main

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/tektoncd/results/pkg/api/server/config"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth"
	authnv1 "k8s.io/api/authentication/v1"
	authzv1 "k8s.io/api/authorization/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stest "k8s.io/client-go/testing"
)

func gatewayRBAC(impersonate bool) (*fake.Clientset, auth.Checker) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "tokenreviews", func(action k8stest.Action) (bool, k8sruntime.Object, error) {
		tr := action.(k8stest.CreateAction).GetObject().(*authnv1.TokenReview)
		if tr.Spec.Token == "authorized" || tr.Spec.Token == "denied" {
			tr.Status.Authenticated = true
			tr.Status.User = authnv1.UserInfo{Username: tr.Spec.Token, UID: "caller-uid", Groups: []string{"callers"}}
		}
		return true, tr, nil
	})
	client.PrependReactor("create", "subjectaccessreviews", func(action k8stest.Action) (bool, k8sruntime.Object, error) {
		sar := action.(k8stest.CreateAction).GetObject().(*authzv1.SubjectAccessReview)
		ra := sar.Spec.ResourceAttributes
		if ra.Verb == "impersonate" {
			sar.Status.Allowed = sar.Spec.User == "authorized" && ra.Name != "denied"
		} else {
			sar.Status.Allowed = (sar.Spec.User == "authorized" || sar.Spec.User == "impersonated") && (ra.Namespace == "foo" || ra.Namespace == "")
		}
		return true, sar, nil
	})
	return client, auth.NewRBAC(client, auth.WithImpersonation(impersonate))
}

func reviewSpecs(t *testing.T, client *fake.Clientset) []string {
	t.Helper()
	var specs []string
	for _, action := range client.Actions() {
		obj := action.(k8stest.CreateAction).GetObject()
		var spec any
		switch obj := obj.(type) {
		case *authnv1.TokenReview:
			spec = obj.Spec
		case *authzv1.SubjectAccessReview:
			spec = obj.Spec
		default:
			t.Fatalf("unexpected authorization action: %T", obj)
		}
		data, err := json.Marshal(spec)
		if err != nil {
			t.Fatal(err)
		}
		specs = append(specs, action.GetResource().Resource+":"+string(data))
	}
	return specs
}

func TestGatewayAuthParity(t *testing.T) {
	for _, tc := range []struct {
		name, token, parent string
		want                int
	}{
		{"missing", "", "foo", 401},
		{"malformed", "invalid", "foo", 401},
		{"invalid", "Bearer invalid", "foo", 401},
		{"allowed", "Bearer authorized", "foo", 200},
		{"denied", "Bearer denied", "foo", 401},
		{"cross namespace", "Bearer authorized", "other", 401},
		{"wildcard", "Bearer authorized", "-", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var baselineSpecs []string
			var baselineBody map[string]any
			for _, baseline := range []bool{true, false} {
				client, checker := gatewayRBAC(false)
				g := newGatewayFixture(t, baseline, &config.Config{}, checker)
				path := "/apis/results.tekton.dev/v1alpha2/parents/" + tc.parent + "/results"
				body := gatewayJSON(t, gatewayRequest(t, g.handler, "GET", path, nil, http.Header{"Authorization": {tc.token}}), tc.want)
				specs := reviewSpecs(t, client)
				if baseline {
					baselineSpecs, baselineBody = specs, body
				} else if !reflect.DeepEqual(baselineSpecs, specs) || !reflect.DeepEqual(baselineBody, body) {
					t.Errorf("auth parity: baseline reviews=%v body=%v; direct reviews=%v body=%v", baselineSpecs, baselineBody, specs, body)
				}
			}
		})
	}
}

func TestGatewayAuthDisabled(t *testing.T) {
	for _, baseline := range []bool{true, false} {
		g := newGatewayFixture(t, baseline, &config.Config{AUTH_DISABLE: true, AUTH_IMPERSONATE: true}, auth.AllowAll{})
		gatewayJSON(t, gatewayRequest(t, g.handler, "GET", resultsURL, nil, http.Header{"Authorization": {"invalid"}, "Impersonate-Group": {"without-user"}}), 200)
	}
}
