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
	"testing"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/google/go-cmp/cmp"
	pagetokenpb "github.com/tektoncd/results/pkg/api/server/v1alpha2/lister/proto/pagetoken_go_proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestEncodeAndDecodePageToken(t *testing.T) {
	pageToken := &pagetokenpb.PageToken{
		Parent: "foo",
		Filter: "summary.status == SUCCESS",
		LastItem: &pagetokenpb.Item{
			Uid: "42",
			OrderBy: &pagetokenpb.Order{
				FieldName: "create_at",
				Value:     timestamppb.New(time.Now()),
				Direction: pagetokenpb.Order_ASC,
			},
		},
	}

	encodedData, err := encodePageToken(pageToken)
	if err != nil {
		t.Fatal(err)
	}

	got, err := decodePageToken(encodedData)
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(pageToken, got,
		cmpopts.IgnoreUnexported(pagetokenpb.PageToken{},
			pagetokenpb.Item{},
			pagetokenpb.Order{},
			timestamppb.Timestamp{})); diff != "" {
		t.Errorf("Mismatch (-want +got):\n%s", diff)
	}
}
