// Copyright 2023 The Tekton Authors
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

package lister

import (
	"encoding/base64"

	pagetokenpb "github.com/tektoncd/results/pkg/api/server/v1alpha2/lister/proto/pagetoken_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// decodePageToken attempts to convert the provided token into a PageToken
// object.
func decodePageToken(in string) (*pagetokenpb.PageToken, error) {
	if in == "" {
		return nil, nil
	}
	decodedData, err := base64.RawURLEncoding.DecodeString(in)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	pageToken := new(pagetokenpb.PageToken)
	if err := proto.Unmarshal(decodedData, pageToken); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return pageToken, nil
}

// encodePageToken turns the PageToken object into a string suitable to be
// delivered by the API.
func encodePageToken(in *pagetokenpb.PageToken) (string, error) {
	wire, err := proto.Marshal(in)
	if err != nil {
		return "", status.Error(codes.Internal, err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(wire), nil
}
