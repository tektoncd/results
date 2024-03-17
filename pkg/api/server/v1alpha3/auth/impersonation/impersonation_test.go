package impersonation

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/metadata"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/kubernetes/fake"
	test "k8s.io/client-go/testing"
)

func TestHeaderMatcher(t *testing.T) {
	for _, tc := range []struct {
		name  string
		key   string
		want  string
		allow bool
	}{
		{
			name:  "impersonation header",
			key:   "Impersonate-User",
			want:  "Impersonate-User",
			allow: true,
		},
		{
			name:  "impersonate extra header",
			key:   "Impersonate-Extra-Scope",
			want:  "Impersonate-Extra-Scope",
			allow: true,
		},
		{
			name:  "grpc metadata header",
			key:   "Grpc-Metadata-Test",
			want:  "Test",
			allow: true,
		},
		{
			name:  "unknown header",
			key:   "Unknown",
			want:  "",
			allow: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, allow := HeaderMatcher(tc.key)
			if got != tc.want || allow != tc.allow {
				t.Errorf("want: %s, got: %s", tc.want, got)
			}
		})
	}
}

func TestNewImpersonation(t *testing.T) {

	t.Run("missing all impersonation header", func(t *testing.T) {
		want := ErrNoImpersonationData
		md := metadata.MD{}
		_, err := NewImpersonation(md)
		if err != want {
			t.Errorf("want: %v, got: %v", want, err)
		}
	})

	t.Run("missing impersonate user header only", func(t *testing.T) {
		want := ErrImpersonateUserRequired
		md := metadata.MD{}
		md.Append("Impersonate-Group", "authorized-group")
		_, err := NewImpersonation(md)
		if err != want {
			t.Errorf("want: %v, got: %v", want, err)
		}
	})

	t.Run("parse impersonation headers", func(t *testing.T) {
		md := metadata.MD{}
		md.Append("Impersonate-User", "authorized-user")
		md.Append("Impersonate-Group", "authorized-group")
		md.Append("Impersonate-Uid", "authorized-uid")
		md.Append("Impersonate-Extra-Scope", "authorized-scope")

		wantResourceAttributes := []authorizationv1.ResourceAttributes{
			{
				Name:     "authorized-user",
				Resource: "users",
				Verb:     "impersonate",
			},
			{
				Name:     "authorized-group",
				Resource: "groups",
				Verb:     "impersonate",
			},
			{
				Group:    "authentication.k8s.io",
				Name:     "authorized-uid",
				Resource: "uids",
				Verb:     "impersonate",
			},
			{
				Group:       "authentication.k8s.io",
				Name:        "authorized-scope",
				Resource:    "userextras",
				Subresource: "scope",
				Verb:        "impersonate",
			},
		}
		wantUserInfo := &user.DefaultInfo{
			Name:   "authorized-user",
			Groups: []string{"authorized-group", "system:authenticated"},
			UID:    "authorized-uid",
			Extra:  map[string][]string{"scope": {"authorized-scope"}},
		}

		got, err := NewImpersonation(md)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(wantResourceAttributes, got.resourceAttributes); diff != "" {
			t.Errorf("-want, +got: %s", diff)
		}
		if diff := cmp.Diff(wantUserInfo, got.userInfo); diff != "" {
			t.Errorf("-want, +got: %s", diff)
		}
	})

}

func TestImpersonation_Check(t *testing.T) {
	k8s := fake.NewSimpleClientset()
	k8s.PrependReactor("create", "subjectaccessreviews", func(action test.Action) (handled bool, ret runtime.Object, err error) {
		sar := action.(test.CreateActionImpl).Object.(*authorizationv1.SubjectAccessReview)
		if sar.Spec.User == "authorized" {
			sar.Status = authorizationv1.SubjectAccessReviewStatus{
				Allowed: true,
			}
		} else {
			sar.Status = authorizationv1.SubjectAccessReviewStatus{
				Denied: true,
			}
		}
		return true, sar, nil
	})

	md := metadata.MD{}
	md.Append("Impersonate-User", "authorized-user")
	md.Append("Impersonate-Group", "authorized-group")
	md.Append("Impersonate-Uid", "authorized-uid")
	md.Append("Impersonate-Extra-Scope", "authorized-scope")

	i, err := NewImpersonation(md)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("authorized user", func(t *testing.T) {
		err := i.Check(context.Background(), k8s.AuthorizationV1(), "authorized")
		if err != nil {
			t.Error(err)
		}
	})

	t.Run("unauthorized user", func(t *testing.T) {
		err := i.Check(context.Background(), k8s.AuthorizationV1(), "unauthorized")
		if err == nil {
			t.Error("Expected unauthorized error")
		}
	})
}
