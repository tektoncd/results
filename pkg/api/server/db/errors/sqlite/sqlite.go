// Package sqlite provides sqlite-specific error checking. This is
// purposefully broken out from the rest of the errors package so that we can
// isolate go-sqlite3's cgo dependency away from the main MySQL based library
// to simplify our testing + deployment.
package sqlite

import (
	"github.com/mattn/go-sqlite3"
	"github.com/tektoncd/results/pkg/api/server/db/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// sqlite converts sqlite3 error codes to gRPC status codes. This is not an
// exhaustive list.
// See https://pkg.go.dev/github.com/mattn/go-sqlite3#pkg-variables for list of
// error codes.
func sqlite(err error) codes.Code {
	serr, ok := err.(sqlite3.Error)
	if !ok {
		return status.Code(err)
	}

	switch serr.Code {
	case sqlite3.ErrConstraint:
		switch serr.ExtendedCode {
		case sqlite3.ErrConstraintUnique:
			return codes.AlreadyExists
		case sqlite3.ErrConstraintForeignKey:
			return codes.FailedPrecondition
		}
		return codes.InvalidArgument
	case sqlite3.ErrNotFound:
		return codes.NotFound
	}
	return status.Code(err)
}

func init() {
	errors.RegisterErrorSpace(sqlite)
}
