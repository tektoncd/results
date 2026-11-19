package postgres

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"
)

func TestTranslate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		err  error
		want codes.Code
	}{
		{
			name: "unique_violation",
			err:  &pgconn.PgError{Code: sqlStateUniqueViolation},
			want: codes.AlreadyExists,
		},
		{
			name: "foreign_key_violation",
			err:  &pgconn.PgError{Code: sqlStateForeignKey},
			want: codes.FailedPrecondition,
		},
		{
			name: "unknown error",
			err:  errors.New("boom"),
			want: codes.Unknown,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := translate(tc.err); got != tc.want {
				t.Fatalf("translate() = %v, want %v", got, tc.want)
			}
		})
	}
}
