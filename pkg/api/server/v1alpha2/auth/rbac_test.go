// Copyright 2021 The Tekton Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auth_test

import (
	"context"
	"fmt"
	"github.com/tektoncd/results/pkg/api/server/config"
	"k8s.io/utils/strings/slices"
	"testing"

	server "github.com/tektoncd/results/pkg/api/server/v1alpha2"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth"
	testclient "github.com/tektoncd/results/pkg/internal/test"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	authnv1 "k8s.io/api/authentication/v1"
	authzv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	test "k8s.io/client-go/testing"
)

func TestRBAC(t *testing.T) {
	// Map of users -> tokens. The 'authorized' user has full permissions.
	users := map[string]string{
		"authorized":   "a",
		"unauthorized": "b",
	}
	k8s := fake.NewSimpleClientset()
	k8s.PrependReactor("create", "tokenreviews", func(action test.Action) (handled bool, ret runtime.Object, err error) {
		tr := action.(test.CreateActionImpl).Object.(*authnv1.TokenReview)
		for user, token := range users {
			if tr.Spec.Token == token {
				tr.Status = authnv1.TokenReviewStatus{
					Authenticated: true,
					User: authnv1.UserInfo{
						Username: user,
					},
				}
				return true, tr, nil
			}
		}
		tr.Status = authnv1.TokenReviewStatus{
			Authenticated: false,
		}
		return true, tr, nil
	})
	k8s.PrependReactor("create", "subjectaccessreviews", func(action test.Action) (handled bool, ret runtime.Object, err error) {
		sar := action.(test.CreateActionImpl).Object.(*authzv1.SubjectAccessReview)
		if sar.Spec.User == "authorized" || slices.Contains(sar.Spec.Groups, "authorized") {
			sar.Status = authzv1.SubjectAccessReviewStatus{
				Allowed: true,
			}
		} else {
			sar.Status = authzv1.SubjectAccessReviewStatus{
				Denied: true,
			}
		}
		return true, sar, nil
	})
	resultsClient, _ := testclient.NewResultsClient(t, &config.Config{}, server.WithAuth(auth.NewRBAC(k8s, auth.WithImpersonation(true))))

	ctx := context.Background()
	result := "foo/results/bar"
	record := "foo/results/bar/records/baz"
	for _, tc := range []struct {
		name             string
		user             string
		token            string
		impersonateUser  string
		impersonateGroup string
		want             codes.Code
	}{
		{
			name:  "authorized user",
			user:  "authorized",
			token: users["authorized"],
			want:  codes.OK,
		},
		{
			name:  "unauthorized user",
			user:  "unauthorized",
			token: users["unauthorized"],
			want:  codes.Unauthenticated,
		},
		{
			name:  "unauthenticated user",
			user:  "unauthenticated",
			token: "",
			want:  codes.Unauthenticated,
		},
		{
			name:            "authorized impersonated user",
			user:            "authorized",
			token:           users["authorized"],
			impersonateUser: "authorized",
			want:            codes.OK,
		},
		{
			name:            "unauthorized impersonated user",
			user:            "authorized",
			token:           users["authorized"],
			impersonateUser: "unauthorized",
			want:            codes.Unauthenticated,
		},
		{
			name:             "authorized impersonated group",
			user:             "authorized",
			token:            users["authorized"],
			impersonateUser:  "unauthorized",
			impersonateGroup: "authorized",
			want:             codes.OK,
		},
		{
			name:             "unauthorized impersonated group",
			user:             "authorized",
			token:            users["authorized"],
			impersonateUser:  "unauthorized",
			impersonateGroup: "unauthorized",
			want:             codes.Unauthenticated,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Simulates a oauth.TokenSource. We avoid using the real
			// oauth.TokenSource here since it requires a higher SecurityLevel
			// + TLS.
			ctx := metadata.AppendToOutgoingContext(ctx, "authorization", fmt.Sprintf("Bearer %s", tc.token))
			if tc.impersonateUser != "" {
				ctx = metadata.AppendToOutgoingContext(ctx, "Impersonate-User", tc.impersonateUser)
			}
			if tc.impersonateGroup != "" {
				ctx = metadata.AppendToOutgoingContext(ctx, "Impersonate-Group", tc.impersonateGroup)
			}
			if _, err := resultsClient.CreateResult(ctx, &pb.CreateResultRequest{
				Parent: "foo",
				Result: &pb.Result{
					Name: "foo/results/bar",
				},
			}); status.Code(err) != tc.want {
				t.Fatalf("CreateResult: %v, want %v", err, tc.want)
			}
			if _, err := resultsClient.GetResult(ctx, &pb.GetResultRequest{Name: result}); status.Code(err) != tc.want {
				t.Fatalf("GetResult: %v, want %v", err, tc.want)
			}
			if _, err := resultsClient.ListResults(ctx, &pb.ListResultsRequest{Parent: "foo"}); status.Code(err) != tc.want {
				t.Fatalf("ListResult: %v, want %v", err, tc.want)
			}
			if _, err := resultsClient.UpdateResult(ctx, &pb.UpdateResultRequest{Name: result, Result: &pb.Result{Name: result}}); status.Code(err) != tc.want {
				t.Fatalf("UpdateResult: %v, want %v", err, tc.want)
			}

			if _, err := resultsClient.CreateRecord(ctx, &pb.CreateRecordRequest{
				Parent: result,
				Record: &pb.Record{
					Name: record,
				},
			}); status.Code(err) != tc.want {
				t.Fatalf("CreateRecord: %v, want %v", err, tc.want)
			}
			if _, err := resultsClient.GetRecord(ctx, &pb.GetRecordRequest{Name: record}); status.Code(err) != tc.want {
				t.Fatalf("GetRecord: %v, want %v", err, tc.want)
			}
			if _, err := resultsClient.ListRecords(ctx, &pb.ListRecordsRequest{Parent: result}); status.Code(err) != tc.want {
				t.Fatalf("ListRecord: %v, want %v", err, tc.want)
			}
			if _, err := resultsClient.UpdateRecord(ctx, &pb.UpdateRecordRequest{Record: &pb.Record{Name: record}}); status.Code(err) != tc.want {
				t.Fatalf("UpdateRecord: %v, want %v", err, tc.want)
			}

			if _, err := resultsClient.DeleteRecord(ctx, &pb.DeleteRecordRequest{Name: record}); status.Code(err) != tc.want {
				t.Fatalf("DeleteRecord: %v, want %v", err, tc.want)
			}
			if _, err := resultsClient.DeleteResult(ctx, &pb.DeleteResultRequest{Name: result}); status.Code(err) != tc.want {
				t.Fatalf("DeleteResult: %v, want %v", err, tc.want)
			}
		})
	}
}
