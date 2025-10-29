// Copyright 2024 The Tekton Authors
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

// Package postgres provides postgres-specific error checking.
package postgres

import (
	"errors"

	dberrors "github.com/tektoncd/results/pkg/api/server/db/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	sqlStateUniqueViolation = "23505"
	sqlStateForeignKey      = "23503"
)

// sqlStateError captures the subset of PgError behavior we rely on.
type sqlStateError interface {
	SQLState() string
}

// translate converts postgres error codes to gRPC status codes.
func translate(err error) codes.Code {
	var sqlErr sqlStateError
	if errors.As(err, &sqlErr) {
		switch sqlErr.SQLState() {
		case sqlStateUniqueViolation:
			return codes.AlreadyExists
		case sqlStateForeignKey:
			return codes.FailedPrecondition
		}
	}
	return status.Code(err)
}

func init() {
	dberrors.RegisterErrorSpace(translate)
}
