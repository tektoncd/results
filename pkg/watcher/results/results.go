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
	"google.golang.org/protobuf/types/known/timestamppb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"knative.dev/pkg/apis"
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

// Object is a union type of different base k8s Object interfaces.
// This is similar in spirit to
// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.4/pkg/client#Object,
// but is defined as its own type to avoid an extra dependency.
type Object interface {
	metav1.Object
	runtime.Object
	StatusConditionGetter
}

type StatusConditionGetter interface {
	GetStatusCondition() apis.ConditionAccessor
}

// Put adds the given Object to the Results API.
// If the parent result is missing or the object is not yet associated with a
// result, one is created automatically.
// If the Object is already associated with a Record, the existing Record is
// updated - otherwise a new Record is created.
func (c *Client) Put(ctx context.Context, o Object, opts ...grpc.CallOption) (*pb.Result, *pb.Record, error) {
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

// ensureResult gets the Result corresponding to the Object, creates a new
// one, or updates the existing Result with new Object details if necessary.
func (c *Client) ensureResult(ctx context.Context, o Object, opts ...grpc.CallOption) (*pb.Result, error) {
	name := resultName(o)
	curr, err := c.ResultsClient.GetResult(ctx, &pb.GetResultRequest{Name: name}, opts...)
	if err != nil && status.Code(err) != codes.NotFound {
		return nil, status.Errorf(status.Code(err), "GetResult(%s): %v", name, err)
	}

	new := &pb.Result{
		Name: name,
	}
	topLevel := isTopLevelRecord(o)
	if topLevel {
		// If the object corresponds to a top level record  - include a RecordSummary.
		new.Summary = &pb.RecordSummary{
			Record:    recordName(name, o),
			Type:      convert.TypeName(o),
			Status:    convert.Status(o.GetStatusCondition()),
			StartTime: getTimestamp(o.GetStatusCondition().GetCondition(apis.ConditionReady)),
			EndTime:   getTimestamp(o.GetStatusCondition().GetCondition(apis.ConditionSucceeded)),
		}
	}

	// Regardless of whether the object is a top level record or not,
	// if the Result doesn't exist yet just create it and return.
	if status.Code(err) == codes.NotFound {
		// Result doesn't exist yet - create.
		req := &pb.CreateResultRequest{
			Parent: o.GetNamespace(),
			Result: new,
		}
		return c.ResultsClient.CreateResult(ctx, req, opts...)
	}

	// From here on, we're checking to see if there are any updates that need
	// to be made to the Record.

	if !topLevel {
		// If the object is top level there's nothing else to do because we
		// won't be modifying the RecordSummary.
		return curr, nil
	}

	// If this object is a top level record, only update if there's been a
	// change to the RecordSummary (only looking at the summary also helps us
	// avoid OUTPUT_ONLY fields in the Result))
	if cmp.Equal(curr.GetSummary(), new.GetSummary(), protocmp.Transform()) {
		// No differences, nothing to do.
		return curr, nil
	}
	req := &pb.UpdateResultRequest{
		Name:   name,
		Result: new,
	}
	return c.ResultsClient.UpdateResult(ctx, req, opts...)
}

func getTimestamp(c *apis.Condition) *timestamppb.Timestamp {
	if c == nil || c.IsFalse() {
		return nil
	}
	return timestamppb.New(c.LastTransitionTime.Inner.Time)
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

func recordName(parent string, o Object) string {
	name, ok := o.GetAnnotations()[annotation.Record]
	if ok {
		return name
	}
	return record.FormatName(parent, defaultName(o))
}

// upsertRecord updates or creates a record for the object. If there has been
// no change in the Record data, the existing Record is returned.
func (c *Client) upsertRecord(ctx context.Context, parent string, o Object, opts ...grpc.CallOption) (*pb.Record, error) {
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
		// Data already exists for the Record - update it iff there is a diff of Data.
		if cmp.Equal(data, curr.GetData(), protocmp.Transform()) {
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

// isTopLevelRecord determines whether an Object is a top level Record - e.g. a
// Record that should be considered the primary record for the result for purposes
// of timing, status, etc. For example, if a Result contains records for a PipelineRun
// and TaskRun, the PipelineRun should take precendence.
// We define an Object to be top level if it does not have any OwnerReferences.
func isTopLevelRecord(o Object) bool {
	return len(o.GetOwnerReferences()) == 0
}
