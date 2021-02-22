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

package results

import (
	"context"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/result"
	"github.com/tektoncd/results/pkg/watcher/convert"
	"github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Client is a wrapper around a Results client that provides helpful utilities
// for performing result operations that require multiple RPCs or data specific
// operations.
type Client struct {
	pb.ResultsClient
}

// NewClient returns a new results client for the particular kind.
func NewClient(client pb.ResultsClient) *Client {
	return &Client{
		ResultsClient: client,
	}
}

// Put adds the given Object to the Results API.
// If the parent result is missing or the object is not yet associated with a
// result, one is created automatically.
// If the Object is already associated with a Record, the existing Record is
// updated - otherwise a new Record is created.
func (c *Client) Put(ctx context.Context, o metav1.Object, opts ...grpc.CallOption) (*pb.Result, *pb.Record, error) {
	// Make sure parent Result exists (or create one)
	result, err := c.ensureResult(ctx, o, opts...)
	if err != nil {
		return nil, nil, err
	}

	// Create or update the record.
	record, err := c.upsertRecord(ctx, result.GetName(), o, opts...)
	if err != nil {
		return nil, nil, err
	}

	return result, record, nil
}

// ensureResult gets the Result corresponding to the Object, or creates a new
// one.
func (c *Client) ensureResult(ctx context.Context, o metav1.Object, opts ...grpc.CallOption) (*pb.Result, error) {
	name := resultName(o)
	res, err := c.ResultsClient.GetResult(ctx, &pb.GetResultRequest{Name: name}, opts...)
	if err != nil && status.Code(err) != codes.NotFound {
		return nil, status.Errorf(status.Code(err), "GetResult(%s): %v", name, err)
	}
	if err == nil {
		return res, nil
	}

	// Result doesn't exist yet - create.
	req := &pb.CreateResultRequest{
		Parent: o.GetNamespace(),
		Result: &pb.Result{
			Name: name,
		},
	}
	return c.ResultsClient.CreateResult(ctx, req, opts...)
}

// resultName gets the result name to use for the given object.
// The name is derived from a known Tekton annotation if available, else
// the object's name is used.
func resultName(o metav1.Object) string {
	// Special case result annotations, since this should already be the
	// full result identifier.
	if v, ok := o.GetAnnotations()[annotation.Result]; ok {
		return v
	}

	var part string
	if v, ok := o.GetLabels()["triggers.tekton.dev/triggers-eventid"]; ok {
		// Don't prefix trigger events. These are 1) not CRD types, 2) are
		// intended to be unique identifiers already, and 3) should be applied
		// to all objects created via trigger templates, so there's no need to
		// prefix these to avoid collision.
		part = v
	} else if len(o.GetOwnerReferences()) > 0 {
		for _, owner := range o.GetOwnerReferences() {
			if strings.EqualFold(owner.Kind, "pipelinerun") {
				part = string(owner.UID)
				break
			}
		}
	}

	if part == "" {
		part = defaultName(o)
	}
	return result.FormatName(o.GetNamespace(), part)
}

// upsertRecord updates or creates a record for the object. If there has been
// no change in the Record data, the existing Record is returned.
func (c *Client) upsertRecord(ctx context.Context, parent string, o metav1.Object, opts ...grpc.CallOption) (*pb.Record, error) {
	name, ok := o.GetAnnotations()[annotation.Record]
	if !ok {
		name = record.FormatName(parent, defaultName(o))
	}

	data, err := convert.ToProto(o)
	if err != nil {
		return nil, err
	}

	curr, err := c.GetRecord(ctx, &pb.GetRecordRequest{Name: name}, opts...)
	if err != nil && status.Code(err) != codes.NotFound {
		return nil, err
	}
	if curr != nil {
		// Data already exists for the Record - update it iff there is a diff.
		if cmp.Equal(curr.Data, data, protocmp.Transform()) {
			// The record data already matches what's stored. Don't update
			// since this will rev update times which throws off resource
			// cleanup checks.
			return curr, nil
		}
		curr.Data = data
		return c.UpdateRecord(ctx, &pb.UpdateRecordRequest{
			Record: curr,
			Etag:   curr.GetEtag(),
		}, opts...)
	}

	// Data does not exist for the Record - create it.
	return c.CreateRecord(ctx, &pb.CreateRecordRequest{
		Parent: parent,
		Record: &pb.Record{
			Name: name,
			Data: data,
		},
	}, opts...)
}

// defaultName is the default Result/Record name that should be used if one is
// not already associated to the Object.
func defaultName(o metav1.Object) string {
	return string(o.GetUID())
}
