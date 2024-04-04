package eventset

import (
	"fmt"
	"regexp"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// DefaultBufferSize is the default buffer size. This based on the recommended
	// gRPC message size for streamed content, which ranges from 16 to 64 KiB. Choosing 32 KiB as a
	// middle ground between the two.
	DefaultBufferSize = 32 * 1024
)

var (
	// NameRegex matches valid name specs for a Result.
	NameRegex = regexp.MustCompile("(^[a-z0-9_-]{1,63})/results/([a-z0-9_-]{1,63})/eventlist/([a-z0-9_-]{1,63}$)")
)

// ParseName splits a full EventList name into its individual (parent, result, name)
// components.
func ParseName(raw string) (parent, result, name string, err error) {
	s := NameRegex.FindStringSubmatch(raw)
	if len(s) != 4 {
		return "", "", "", status.Errorf(codes.InvalidArgument, "name must match %s", NameRegex.String())
	}
	return s[1], s[2], s[3], nil
}

// FormatName takes in a parent ("a/results/b") and record name ("c") and
// returns the full resource name ("a/results/b/eventset/c").
func FormatName(parent, name string) string {
	return fmt.Sprintf("%s/eventlist/%s", parent, name)
}
