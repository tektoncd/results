// Package protoutil provides utilities for manipulating protos in tests.
package protoutil

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// Any wraps a proto message in an Any proto, or causes the test to fail.
func Any(t testing.TB, m proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(m)
	if err != nil {
		t.Fatalf("error wrapping Any proto: %v", err)
	}
	return a
}

// AnyBytes returns the marshalled bytes of an Any proto wrapping the given
// message, or causes the test to fail.
func AnyBytes(t testing.TB, m proto.Message) []byte {
	t.Helper()
	b, err := proto.Marshal(Any(t, m))
	if err != nil {
		t.Fatalf("error marshalling Any proto: %v", err)
	}
	return b
}
