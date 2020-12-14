// Package result provides utilities for manipulating and validating Results.
package result

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"

	"github.com/google/cel-go/cel"
	"github.com/tektoncd/results/pkg/api/server/db"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
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

// ToStorage converts an API Result into its corresponding database storage
// equivalent.
func ToStorage(r *pb.Result) (*db.Result, error) {
	parent, name, err := ParseName(r.GetName())
	if err != nil {
		return nil, err
	}
	ann, err := json.Marshal(r.GetAnnotations())
	if err != nil {
		return nil, err
	}
	result := &db.Result{
		Parent:      parent,
		ID:          r.GetId(),
		Name:        name,
		UpdatedTime: r.UpdatedTime.AsTime(),
		CreatedTime: r.CreatedTime.AsTime(),
		Annotations: ann,
	}
	return result, nil
}

// ToAPI converts a database storage Result into its corresponding API
// equivalent.
func ToAPI(r *db.Result) *pb.Result {
	ann := map[string]string{}
	if err := json.Unmarshal(r.Annotations, &ann); err != nil {
		ann = nil
	}
	return &pb.Result{
		Name:        fmt.Sprintf("%s/results/%s", r.Parent, r.Name),
		Id:          r.ID,
		Annotations: ann,
		CreatedTime: timestamppb.New(r.CreatedTime),
		UpdatedTime: timestamppb.New(r.UpdatedTime),
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

// GetResultByName is the helper function to get a Result by its parent and name
func GetResultByName(gdb *gorm.DB, name string) (*pb.Result, error) {
	parent, name, err := ParseName(name)
	if err != nil {
		return nil, err
	}
	var results []*db.Result = []*db.Result{}
	if err := gdb.Where("parent = ?", parent).Where("name = ?", name).Find(&results).Error; err != nil {
		log.Printf("failed to query on database: %v", err)
		return nil, fmt.Errorf("failed to query on a result: %w", err)
	}
	if len(results) == 0 {
		return nil, status.Error(codes.NotFound, "result not found")
	}
	if len(results) > 1 {
		log.Println("Warning: multiple rows found")
	}
	return ToAPI(results[0]), nil
}
