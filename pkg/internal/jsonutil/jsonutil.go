package jsonutil

import (
	"encoding/json"
	"testing"
)

// AnyBytes returns the marshalled bytes of an Any proto wrapping the given
// message, or causes the test to fail.
func AnyBytes(t testing.TB, i interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(i)
	if err != nil {
		t.Fatalf("error marshalling Any proto: %v", err)
	}
	return b
}
