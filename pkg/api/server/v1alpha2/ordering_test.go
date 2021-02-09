package server

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/pipeline/test/diff"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestOrderBy(t *testing.T) {
	for _, tc := range []struct {
		in  string
		out string
	}{{
		in:  "created_time DesC,updated_time aSc",
		out: "created_time DESC,updated_time ASC",
	}, {
		in:  "   created_time    DesC   , updated_time deSC",
		out: "created_time DESC,updated_time DESC",
	}} {
		ob, err := orderBy(tc.in)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if ob != tc.out {
			t.Errorf(diff.PrintWantGot(cmp.Diff(tc.out, ob)))
		}
	}
}

func TestOrderByInvalidArguments(t *testing.T) {
	for _, tc := range []struct {
		in string
	}{{
		in: "unsupported_field_name",
	}, {
		in: "a string of many words",
	}, {
		in: "current_time invalid_sort_order",
	}, {
		in: "x current_time asc",
	}, {
		in: " current_time asc x ",
	}} {
		_, err := orderBy(tc.in)
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected error code %d received %d (error: %v)", codes.InvalidArgument, status.Code(err), err)
		}
	}
}

func TestNormalizeOrderByField(t *testing.T) {
	for _, tc := range []struct {
		in  string
		out string
	}{{
		in:  "created_time",
		out: "created_time",
	}, {
		in:  "created_time asc",
		out: "created_time ASC",
	}, {
		in:  "   created_time    DesC   ",
		out: "created_time DESC",
	}} {
		n, err := normalizeOrderByField(tc.in)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if n != tc.out {
			t.Errorf(diff.PrintWantGot(cmp.Diff(tc.out, n)))
		}
	}
}

func TestIsAllowedField(t *testing.T) {
	if isAllowedField("not_an_allowed_field") {
		t.Errorf("field unexpectedly allowed")
	}
	for _, s := range allowedOrderByFields {
		if !isAllowedField(s) {
			t.Errorf("%s expected to be allowed field", s)
		}
	}
}

func TestOrderByDirection(t *testing.T) {
	for _, tc := range []struct {
		field     string
		direction string
		out       string
	}{{
		field:     "foo",
		direction: "asC",
		out:       "foo ASC",
	}, {
		field:     "foo",
		direction: "deSc",
		out:       "foo DESC",
	}} {
		s, err := orderByDirection(tc.field, tc.direction)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if s != tc.out {
			t.Errorf("expected %s received %s", tc.out, s)
		}
	}
}
