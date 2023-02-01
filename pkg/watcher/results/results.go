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
	"errors"
	"fmt"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/result"
	"github.com/tektoncd/results/pkg/watcher/convert"
	"github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/logging"
)

// Sentinel error that signals to callers that a given result hasn't been
// created yet.
var ErrResultNotCreatedYet = errors.New("result not created yet")

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
// If the parent result is missing and the object in question is a top-level
// (i.e. isn't owned by another object) the result is created
// automatically. Otherwise, the method returns an ErrResultNotCreatedYet error,
// by signaling to the caller that the result isn't yet available (the
// controller must requeue the object to wait until the parent resource creates
// the result).
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

// ensureResult gets the Result corresponding to the Object, creates a new one,
// or updates the existing Result with new Object details if necessary. It
// returns an ErrResultNotCreatedYet error if the parent result is missing and
// the provided object isn't a top-level one (i.e. is owned by another object).
func (c *Client) ensureResult(ctx context.Context, o Object, opts ...grpc.CallOption) (*pb.Result, error) {
	// Whether or not the object in question owns others.
	topLevel := isTopLevelRecord(o)
	logger := logging.FromContext(ctx).With()
	resultName := resultName(o)
	curr, err := c.ResultsClient.GetResult(ctx, &pb.GetResultRequest{Name: resultName}, opts...)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			if !topLevel {
				// Signal to the controller that the result
				// isn't available yet. We'll defer the process
				// to wait until the top-level object creates
				// the result instead of attempting to create it
				// now. This avoid unnecessary requests to the
				// API server that in highly concurrent
				// scenarios wil result in a bunch of unique
				// constraint violations.
				logger.Debug("result not created yet - deferring creation to the parent resource")
				return nil, fmt.Errorf("%s: %w", resultName, ErrResultNotCreatedYet)
			}
		} else {
			// Unrecoverable error.
			return nil, status.Errorf(status.Code(err), "GetResult(%s): %v", resultName, err)
		}
	}

	// If the object in question isn't a top-level, simply return the result to the caller.
	if !topLevel {
		return curr, nil
	}

	recordName := recordName(resultName, o)
	logger = logger.With(zap.String(annotation.Result, resultName),
		zap.String(annotation.Record, recordName))

	new := &pb.Result{
		Name: resultName,
		Summary: &pb.RecordSummary{
			Record:    recordName,
			Type:      convert.TypeName(o),
			Status:    convert.Status(o.GetStatusCondition()),
			StartTime: getTimestamp(o.GetStatusCondition().GetCondition(apis.ConditionReady)),
			EndTime:   getTimestamp(o.GetStatusCondition().GetCondition(apis.ConditionSucceeded)),
		},
	}

	if curr == nil {
		logger.Debug("Result doesn't exist yet - creating")
		req := &pb.CreateResultRequest{
			Parent: parentName(o),
			Result: new,
		}
		return c.ResultsClient.CreateResult(ctx, req, opts...)
	}

	// From here on, we're checking to see if there are any updates that need
	// to be made to the Record.
	if cmp.Equal(curr.GetSummary(), new.GetSummary(), protocmp.Transform()) {
		logger.Debug("No further actions to be done on the Result: no differences found")
		return curr, nil
	}
	logger.Debug("Updating Result")
	req := &pb.UpdateResultRequest{
		Name:   resultName,
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
	// Attempt to read the record name from annotations only if the object
	// in question is a top-level record (i.e. it isn't owned by another
	// object). Otherwise, the annotation containing the record name maybe
	// was propagated by the owner what causes conflicts while upserting the
	// object into the API. For further details, please see
	// https://github.com/tektoncd/results/issues/296.
	if isTopLevelRecord(o) {
		if name, ok := o.GetAnnotations()[annotation.Record]; ok {
			return name
		}
	}
	return record.FormatName(parent, defaultName(o))
}

// parentName returns the parent's name of the result in question. If the
// results annotation is set, returns the first segment of the result
// name. Otherwise, returns the object's namespace.
func parentName(o metav1.Object) string {
	if value, found := o.GetAnnotations()[annotation.Result]; found {
		if parts := strings.Split(value, "/"); len(parts) != 0 {
			return parts[0]
		}
	}
	return o.GetNamespace()
}

// upsertRecord updates or creates a record for the object. If there has been
// no change in the Record data, the existing Record is returned.
func (c *Client) upsertRecord(ctx context.Context, parent string, o Object, opts ...grpc.CallOption) (*pb.Record, error) {
	recordName := recordName(parent, o)
	logger := logging.FromContext(ctx).With(zap.String(annotation.Record, recordName))
	data, err := convert.ToProto(o)
	if err != nil {
		return nil, err
	}

	curr, err := c.GetRecord(ctx, &pb.GetRecordRequest{Name: recordName}, opts...)
	if err != nil && status.Code(err) != codes.NotFound {
		return nil, err
	}
	if curr != nil {
		// Data already exists for the Record - update it iff there is a diff of Data.
		if cmp.Equal(data, curr.GetData(), protocmp.Transform()) {
			logger.Debug("No further actions to be done on the Record: no changes found")
			return curr, nil
		}

		logger.Debug("Updating Record")
		curr.Data = data
		return c.UpdateRecord(ctx, &pb.UpdateRecordRequest{
			Record: curr,
			Etag:   curr.GetEtag(),
		}, opts...)
	}

	logger.Debug("Record doesn't exist yet - creating")
	return c.CreateRecord(ctx, &pb.CreateRecordRequest{
		Parent: parent,
		Record: &pb.Record{
			Name: recordName,
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
