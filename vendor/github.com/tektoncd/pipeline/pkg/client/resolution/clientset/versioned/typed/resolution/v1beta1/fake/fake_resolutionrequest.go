/*
Copyright 2020 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1beta1 "github.com/tektoncd/pipeline/pkg/apis/resolution/v1beta1"
	resolutionv1beta1 "github.com/tektoncd/pipeline/pkg/client/resolution/clientset/versioned/typed/resolution/v1beta1"
	gentype "k8s.io/client-go/gentype"
)

// fakeResolutionRequests implements ResolutionRequestInterface
type fakeResolutionRequests struct {
	*gentype.FakeClientWithList[*v1beta1.ResolutionRequest, *v1beta1.ResolutionRequestList]
	Fake *FakeResolutionV1beta1
}

func newFakeResolutionRequests(fake *FakeResolutionV1beta1, namespace string) resolutionv1beta1.ResolutionRequestInterface {
	return &fakeResolutionRequests{
		gentype.NewFakeClientWithList[*v1beta1.ResolutionRequest, *v1beta1.ResolutionRequestList](
			fake.Fake,
			namespace,
			v1beta1.SchemeGroupVersion.WithResource("resolutionrequests"),
			v1beta1.SchemeGroupVersion.WithKind("ResolutionRequest"),
			func() *v1beta1.ResolutionRequest { return &v1beta1.ResolutionRequest{} },
			func() *v1beta1.ResolutionRequestList { return &v1beta1.ResolutionRequestList{} },
			func(dst, src *v1beta1.ResolutionRequestList) { dst.ListMeta = src.ListMeta },
			func(list *v1beta1.ResolutionRequestList) []*v1beta1.ResolutionRequest {
				return gentype.ToPointerSlice(list.Items)
			},
			func(list *v1beta1.ResolutionRequestList, items []*v1beta1.ResolutionRequest) {
				list.Items = gentype.FromPointerSlice(items)
			},
		),
		fake,
	}
}
