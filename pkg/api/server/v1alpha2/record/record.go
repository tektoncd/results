// Package record provides utilities for manipulating and validating Records.
package record

import (
	"fmt"
	"regexp"

	"github.com/tektoncd/results/pkg/api/server/db"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
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
func ToStorage(parent, resultName, resultID, name string, r *pb.Record) (*db.Record, error) {
	data, err := proto.Marshal(r.Data)
	if err != nil {
		return nil, err
	}
	return &db.Record{
		Parent:     parent,
		ResultName: resultName,
		ResultID:   resultID,

		ID:   r.GetId(),
		Name: name,

		Data: data,
	}, nil
}

// ToAPI converts a database storage Record into its corresponding API
// equivalent.
func ToAPI(r *db.Record) (*pb.Record, error) {
	out := &pb.Record{
		Name: fmt.Sprintf("%s/results/%s/records/%s", r.Parent, r.ResultName, r.Name),
		Id:   r.ID,
	}

	// Check if data was stored before unmarshalling, to avoid returning `{}`.
	if r.Data != nil {
		any := new(anypb.Any)
		if err := proto.Unmarshal(r.Data, any); err != nil {
			return nil, err
		}
		out.Data = any
	}

	return out, nil
}
