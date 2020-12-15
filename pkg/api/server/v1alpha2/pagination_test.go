package server

import (
	"fmt"
	"testing"
)

func TestPageSize(t *testing.T) {
	for _, tc := range []struct {
		in   int
		want int
		err  bool
	}{
		{
			in:   1,
			want: 1,
		},
		{
			in:  -1,
			err: true,
		},
		{
			in:   int(^uint32(0) >> 1), // Max int32
			want: maxPageSize,
		},
	} {
		t.Run(fmt.Sprintf("%d", tc.in), func(t *testing.T) {
			got, err := pageSize(tc.in)
			if got != tc.want || (err == nil && tc.err) {
				t.Errorf("want (%d, %t), got (%d, %v)", tc.want, tc.err, got, err)
			}
		})
	}
}

func TestPageStart(t *testing.T) {
	for _, tc := range []struct {
		name   string
		token  string
		filter string
		want   string
		err    bool
	}{
		{
			name:   "success",
			token:  pagetoken(t, "a", "b"),
			filter: "b",
			want:   "a",
		},
		{
			name:  "wrong filter",
			token: pagetoken(t, "a", "c"),
			err:   true,
		},
		{
			name:  "invalid token",
			token: "tacocat",
			err:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pageStart(tc.token, tc.filter)
			if got != tc.want || (err == nil && tc.err) {
				t.Errorf("want (%s, %t), got (%s, %v)", tc.want, tc.err, got, err)
			}
		})
	}
}
