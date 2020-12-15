// Package result provides utilities for manipulating and validating Results.
package result

import (
	"fmt"
	"log"
	"regexp"

	"github.com/google/cel-go/cel"
	"github.com/tektoncd/results/pkg/api/server/db"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	// NameRegex matches valid name specs for a Result.
	NameRegex = regexp.MustCompile("(^[a-z0-9_-]{1,63})/results/([a-z0-9_-]{1,63}$)")
)

// ParseName splits a full Result name into its individual (parent, name)
// components.
func ParseName(raw string) (parent, name string, err error) {
	s := NameRegex.FindStringSubmatch(raw)
	if len(s) != 3 {
		return "", "", fmt.Errorf("name must match %s", NameRegex.String())
	}
	return s[1], s[2], nil
}

// FormatName takes in a parent ("a") and result name ("b") and
// returns the full resource name ("a/results/b").
func FormatName(parent, name string) string {
	return fmt.Sprintf("%s/results/%s", parent, name)
}

// ToStorage converts an API Result into its corresponding database storage
// equivalent.
// parent,name should be the name parts (e.g. not containing "/results/").
func ToStorage(r *pb.Result) (*db.Result, error) {
	parent, name, err := ParseName(r.GetName())
	if err != nil {
		return nil, err
	}
	result := &db.Result{
		Parent:      parent,
		ID:          r.GetId(),
		Name:        name,
		UpdatedTime: r.UpdatedTime.AsTime(),
		CreatedTime: r.CreatedTime.AsTime(),
		Annotations: r.Annotations,
	}
	return result, nil
}

// ToAPI converts a database storage Result into its corresponding API
// equivalent.
func ToAPI(r *db.Result) *pb.Result {
	return &pb.Result{
		Name:        FormatName(r.Parent, r.Name),
		Id:          r.ID,
		CreatedTime: timestamppb.New(r.CreatedTime),
		UpdatedTime: timestamppb.New(r.UpdatedTime),
		Annotations: r.Annotations,
	}
}

// Match determines whether the given CEL filter matches the result.
func Match(r *pb.Result, prg cel.Program) (bool, error) {
	if prg == nil {
		return true, nil
	}
	if r == nil {
		return false, nil
	}

	out, _, err := prg.Eval(map[string]interface{}{
		"result": r,
	})
	if err != nil {
		log.Printf("failed to evaluate the expression: %v", err)
		return false, status.Errorf(codes.InvalidArgument, "failed to evaluate filter: %v", err)
	}
	b, ok := out.Value().(bool)
	if !ok {
		return false, status.Errorf(codes.InvalidArgument, "expected boolean result, got %s", out.Type().TypeName())
	}
	return b, nil
}
