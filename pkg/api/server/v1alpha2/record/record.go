// Copyright 2020 The Tekton Authors
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

// Package record provides utilities for manipulating and validating Records.
package record

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/tektoncd/results/pkg/api/server/config"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/log"
	"github.com/tektoncd/results/pkg/apis/v1alpha2"

	"github.com/google/cel-go/cel"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	resultscel "github.com/tektoncd/results/pkg/api/server/cel"
	"github.com/tektoncd/results/pkg/api/server/db"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	typeSize = 768
)

var (
	// NameRegex matches valid name specs for a Result.
	NameRegex = regexp.MustCompile("(^[a-z0-9_-]{1,63})/results/([a-z0-9_-]{1,63})/records/([a-z0-9_-]{1,63}$)")
)

// ParseName splits a full Result name into its individual (parent, result, name)
// components.
func ParseName(raw string) (parent, result, name string, err error) {
	s := NameRegex.FindStringSubmatch(raw)
	if len(s) != 4 {
		return "", "", "", status.Errorf(codes.InvalidArgument, "name must match %s", NameRegex.String())
	}
	return s[1], s[2], s[3], nil
}

// FormatName takes in a parent ("a/results/b") and record name ("c") and
// returns the full resource name ("a/results/b/records/c").
func FormatName(parent, name string) string {
	return fmt.Sprintf("%s/records/%s", parent, name)
}

// ToStorage converts an API Record into its corresponding database storage
// equivalent.
// parent,result,name should be the name parts (e.g. not containing "/results/" or "/records/").
func ToStorage(parent, resultName, resultID, name string, r *pb.Record, config *config.Config) (*db.Record, error) {
	if err := validateData(r.GetData()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	id := r.GetUid()
	if id == "" {
		id = r.GetId()
	}

	dbr := &db.Record{
		Parent:     parent,
		ResultName: resultName,
		ResultID:   resultID,

		ID:   id,
		Name: name,

		Type: r.GetData().GetType(),
		Data: r.GetData().GetValue(),

		Etag: r.Etag,
	}

	if r.CreatedTime.IsValid() {
		dbr.CreatedTime = r.CreatedTime.AsTime()
	}
	if r.CreateTime.IsValid() {
		dbr.CreatedTime = r.CreateTime.AsTime()
	}
	if r.UpdatedTime.IsValid() {
		dbr.UpdatedTime = r.UpdatedTime.AsTime()
	}
	if r.UpdateTime.IsValid() {
		dbr.UpdatedTime = r.UpdateTime.AsTime()
	}
	if dbr.Type == v1alpha2.LogRecordType {
		data, err := log.ToStorage(r, config)
		if err != nil {
			return nil, err
		}
		dbr.Data = data
	}

	return dbr, nil
}

// ToAPI converts a database storage Record into its corresponding API
// equivalent.
func ToAPI(r *db.Record) *pb.Record {
	out := &pb.Record{
		Name: fmt.Sprintf("%s/results/%s/records/%s", r.Parent, r.ResultName, r.Name),
		Id:   r.ID,
		Uid:  r.ID,
		Etag: r.Etag,
	}

	if !r.CreatedTime.IsZero() {
		out.CreatedTime = timestamppb.New(r.CreatedTime)
		out.CreateTime = timestamppb.New(r.CreatedTime)
	}
	if !r.UpdatedTime.IsZero() {
		out.UpdatedTime = timestamppb.New(r.UpdatedTime)
		out.UpdateTime = timestamppb.New(r.UpdatedTime)
	}

	if r.Data != nil {
		out.Data = &pb.Any{
			Type:  r.Type,
			Value: r.Data,
		}
	}

	return out
}

// Match determines whether the given CEL filter matches the result.
func Match(r *pb.Record, prg cel.Program) (bool, error) {
	if r == nil {
		return false, nil
	}

	var m map[string]interface{}
	if d := r.GetData().GetValue(); d != nil {
		if err := json.Unmarshal(r.GetData().GetValue(), &m); err != nil {
			return false, err
		}
	}

	return resultscel.Match(prg, map[string]interface{}{
		"name":      r.GetName(),
		"data_type": r.GetData().GetType(),
		"data":      m,
	})
}

// UpdateEtag updates the etag field of a record according to its content.
// The record should at least have its `Id` and `UpdatedTime` fields set.
func UpdateEtag(r *db.Record) error {
	if r.ID == "" {
		return fmt.Errorf("the ID field must be set")
	}
	if r.UpdatedTime.IsZero() {
		return status.Error(codes.Internal, "the UpdatedTime field must be set")
	}
	r.Etag = fmt.Sprintf("%s-%v", r.ID, r.UpdatedTime.UnixNano())
	return nil
}

func validateData(m *pb.Any) error {
	if err := ValidateType(m.GetType()); err != nil {
		return err
	}

	if m == nil {
		return nil
	}
	switch m.GetType() {
	case "pipeline.tekton.dev/TaskRun":
		return json.Unmarshal(m.GetValue(), &v1beta1.TaskRun{})
	case "pipeline.tekton.dev/PipelineRun":
		return json.Unmarshal(m.GetValue(), &v1beta1.PipelineRun{})
	case "results.tekton.dev/v1alpha2.Log":
		return json.Unmarshal(m.GetValue(), &v1alpha2.Log{})
	default:
		// If it's not a well known type, just check that the message is a valid JSON document.
		return json.Unmarshal(m.GetValue(), &json.RawMessage{})
	}
}

func ValidateType(t string) error {
	// Certain DBs like sqlite will massage CHAR types to TEXT, so enforce
	// this in our code for consistency.
	if len(t) > typeSize {
		return status.Errorf(codes.InvalidArgument, "type must not exceed %d characters", typeSize)
	}
	return nil
}
