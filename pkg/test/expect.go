package test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

// AssertOutput is a helper function to assert that the actual output matches the expected output.
func AssertOutput(t *testing.T, expected, actual interface{}) {
	t.Helper()
	diff := cmp.Diff(actual, expected)
	if diff == "" {
		return
	}

	t.Errorf(`
Unexpected output:
%s

Expected
%s

Actual
%s
`, diff, expected, actual)
}
