// Package protoutil provides utilities for manipulating protos in tests.
package protoutil

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// Any wraps a proto message in an Any proto. If there is any problem,
// this panics.
func Any(m proto.Message) *anypb.Any {
	a, err := anypb.New(m)
	if err != nil {
		panic(err)
	}
	return a
}

// AnyBytes returns the marshalled bytes of an Any proto wrapping the given
// message. If there is any problem, this panics.
func AnyBytes(m proto.Message) []byte {
	b, err := proto.Marshal(Any(m))
	if err != nil {
		panic(err)
	}
	return b
}
