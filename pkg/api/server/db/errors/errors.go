package errors

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

var (
	errorSpace ErrorSpace
)

// ErrorSpace allows implementations to inject database specific error checking
// to the application.
type ErrorSpace func(error) codes.Code

// RegisterErrorSpace registers the ErrorSpace - last one wins.
func RegisterErrorSpace(f ErrorSpace) {
	errorSpace = f
}

// Wrap converts database error codes into their corresponding gRPC status
// codes.
func Wrap(err error) error {
	if err == nil {
		return err
	}

	// Check for gorm provided errors first - these are more likely to be
	// supported across drivers.
	if code, ok := gormCode(err); ok {
		return status.Error(code, err.Error())
	}

	// Fallback to implementation specific codes.
	if errorSpace != nil {
		return status.Error(errorSpace(err), err.Error())
	}

	return err
}

// gormCode returns gRPC status codes corresponding to gorm errors. This is not
// an exhaustive list.
// See https://pkg.go.dev/gorm.io/gorm@v1.20.7#pkg-variables for list of
// errors.
func gormCode(err error) (codes.Code, bool) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return codes.NotFound, true
	}
	return codes.Unknown, false
}
