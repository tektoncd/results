//go:build e2e
// +build e2e

// Copyright 2026 The Tekton Authors
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

package db_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	model "github.com/tektoncd/results/pkg/api/server/db"
	dberrors "github.com/tektoncd/results/pkg/api/server/db/errors"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestErrorMapping_UniqueViolationResult verifies that creating a Result
// with a duplicate name through the deployed API returns codes.AlreadyExists.
//
// Path exercised: gRPC request → API server → GORM insert → Postgres returns
// pgconn.PgError SQLSTATE 23505 → errors.Wrap → translate() → AlreadyExists.
// SQLite unit tests can never reach translate() because SQLite returns a
// different error type that does not implement the SQLState() interface.
func TestErrorMapping_UniqueViolationResult(t *testing.T) {
	ctx := context.Background()
	const parent = "db-err-dup-res"

	req := &pb.CreateResultRequest{
		Parent: parent,
		Result: &pb.Result{
			Name: parent + "/results/dup-result",
		},
	}

	res, err := adminClient.CreateResult(ctx, req)
	if err != nil {
		t.Fatalf("first CreateResult failed: %v", err)
	}
	t.Cleanup(func() { deleteResult(t, res.GetName()) })

	_, err = adminClient.CreateResult(ctx, req)
	if err == nil {
		t.Fatal("expected error on duplicate Result, got nil")
	}
	if got := status.Code(err); got != codes.AlreadyExists {
		t.Errorf("want gRPC code %s, got %s (err: %v)", codes.AlreadyExists, got, err)
	}
}

// TestErrorMapping_UniqueViolationRecord verifies duplicate Record detection
// through the deployed API against real Postgres.
func TestErrorMapping_UniqueViolationRecord(t *testing.T) {
	ctx := context.Background()
	const parent = "db-err-dup-rec"

	res, err := adminClient.CreateResult(ctx, &pb.CreateResultRequest{
		Parent: parent,
		Result: &pb.Result{Name: parent + "/results/parent-for-dup-rec"},
	})
	if err != nil {
		t.Fatalf("CreateResult failed: %v", err)
	}
	t.Cleanup(func() { deleteResult(t, res.GetName()) })

	recReq := &pb.CreateRecordRequest{
		Parent: res.GetName(),
		Record: &pb.Record{
			Name: res.GetName() + "/records/dup-record",
			Data: &pb.Any{
				Type:  "testing.tekton.dev/test",
				Value: mustJSON(t, map[string]string{"key": "value"}),
			},
		},
	}

	if _, err := adminClient.CreateRecord(ctx, recReq); err != nil {
		t.Fatalf("first CreateRecord failed: %v", err)
	}

	_, err = adminClient.CreateRecord(ctx, recReq)
	if err == nil {
		t.Fatal("expected error on duplicate Record, got nil")
	}
	if got := status.Code(err); got != codes.AlreadyExists {
		t.Errorf("want gRPC code %s, got %s (err: %v)", codes.AlreadyExists, got, err)
	}
}

// TestErrorMapping_ForeignKeyViolation verifies that Postgres SQLSTATE 23503
// (foreign_key_violation) is translated to codes.FailedPrecondition.
//
// The server's CreateRecord pre-checks the parent via getResultID, returning
// NotFound before the INSERT ever runs. To hit the actual FK constraint we
// must bypass the server and INSERT directly into the records table via the
// raw DB connection, then wrap the error through dberrors.Wrap.
func TestErrorMapping_ForeignKeyViolation(t *testing.T) {
	orphan := &model.Record{
		Parent:      "db-err-fk",
		ResultID:    "nonexistent-result-id",
		ResultName:  "nonexistent-result",
		ID:          "orphan-record-id",
		Name:        "orphan-record",
		Type:        "testing.tekton.dev/test",
		Data:        []byte(`{"orphan":true}`),
		Etag:        "test",
		CreatedTime: time.Now(),
		UpdatedTime: time.Now(),
	}

	t.Cleanup(func() { rawDB.Delete(orphan) })

	err := rawDB.Create(orphan).Error
	if err == nil {
		t.Fatal("expected FK violation error from direct insert, got nil")
	}

	wrapped := dberrors.Wrap(err)
	if got := status.Code(wrapped); got != codes.FailedPrecondition {
		t.Errorf("want gRPC code %s for FK violation, got %s (raw err: %v)",
			codes.FailedPrecondition, got, err)
	}
}

// TestErrorMapping_NotFound verifies that fetching a non-existent Result
// through the deployed API returns codes.NotFound.
func TestErrorMapping_NotFound(t *testing.T) {
	ctx := context.Background()

	_, err := readClient.GetResult(ctx, &pb.GetResultRequest{
		Name: "db-err-notfound/results/does-not-exist",
	})
	if err == nil {
		t.Fatal("expected NotFound error, got nil")
	}
	if got := status.Code(err); got != codes.NotFound {
		t.Errorf("want gRPC code %s, got %s (err: %v)", codes.NotFound, got, err)
	}
}

// TestErrorMapping_StaleEtag verifies that updating a Result with a stale
// etag through the deployed API returns codes.FailedPrecondition.
func TestErrorMapping_StaleEtag(t *testing.T) {
	ctx := context.Background()
	const parent = "db-err-etag"

	res, err := adminClient.CreateResult(ctx, &pb.CreateResultRequest{
		Parent: parent,
		Result: &pb.Result{
			Name:        parent + "/results/etag-test",
			Annotations: map[string]string{"v": "1"},
		},
	})
	if err != nil {
		t.Fatalf("CreateResult failed: %v", err)
	}
	t.Cleanup(func() { deleteResult(t, res.GetName()) })

	_, err = adminClient.UpdateResult(ctx, &pb.UpdateResultRequest{
		Name: res.GetName(),
		Result: &pb.Result{
			Name:        res.GetName(),
			Annotations: map[string]string{"v": "2"},
		},
		Etag: "deliberately-wrong-etag",
	})
	if err == nil {
		t.Fatal("expected FailedPrecondition for stale etag, got nil")
	}
	if got := status.Code(err); got != codes.FailedPrecondition {
		t.Errorf("want gRPC code %s, got %s (err: %v)", codes.FailedPrecondition, got, err)
	}
}

// TestErrorMapping_CascadeDelete verifies ON DELETE CASCADE through the
// deployed API: deleting a Result must also delete its child Records.
func TestErrorMapping_CascadeDelete(t *testing.T) {
	ctx := context.Background()
	const parent = "db-err-cascade"

	res, err := adminClient.CreateResult(ctx, &pb.CreateResultRequest{
		Parent: parent,
		Result: &pb.Result{Name: parent + "/results/cascade-parent"},
	})
	if err != nil {
		t.Fatalf("CreateResult failed: %v", err)
	}

	rec, err := adminClient.CreateRecord(ctx, &pb.CreateRecordRequest{
		Parent: res.GetName(),
		Record: &pb.Record{
			Name: res.GetName() + "/records/cascade-child",
			Data: &pb.Any{
				Type:  "testing.tekton.dev/test",
				Value: mustJSON(t, map[string]string{"cascade": "true"}),
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateRecord failed: %v", err)
	}

	if _, err := adminClient.DeleteResult(ctx, &pb.DeleteResultRequest{
		Name: res.GetName(),
	}); err != nil {
		t.Fatalf("DeleteResult failed: %v", err)
	}

	_, err = readClient.GetRecord(ctx, &pb.GetRecordRequest{Name: rec.GetName()})
	if err == nil {
		t.Fatal("expected Record to be deleted via cascade, but GetRecord succeeded")
	}
	if got := status.Code(err); got != codes.NotFound {
		t.Errorf("want gRPC code %s after cascade delete, got %s (err: %v)",
			codes.NotFound, got, err)
	}
}

// deleteResult is a best-effort cleanup helper.
func deleteResult(t *testing.T, name string) {
	t.Helper()
	_, _ = adminClient.DeleteResult(context.Background(), &pb.DeleteResultRequest{Name: name})
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return b
}
