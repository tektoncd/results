package jsonutil

import (
	"encoding/json"
	"testing"
)

// AnyBytes returns the marshalled bytes of an Any proto wrapping the given
// message, or causes the test to fail.
func AnyBytes(tb testing.TB, i interface{}) []byte {
	tb.Helper()
	b, err := json.Marshal(i)
	if err != nil {
		tb.Fatalf("error marshalling Any proto: %v", err)
	}
	return b
}
