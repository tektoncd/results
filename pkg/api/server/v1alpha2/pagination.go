package server

import (
	"fmt"

	"github.com/tektoncd/results/pkg/api/server/db/pagination"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	minPageSize = 50
	maxPageSize = 10000
)

func pageSize(in int) (int, error) {
	if in < 0 {
		return 0, status.Error(codes.InvalidArgument, "PageSize should be greater than 0")
	} else if in == 0 {
		return minPageSize, nil
	} else if in > maxPageSize {
		return maxPageSize, nil
	}
	return in, nil
}

func pageStart(token, filter string) (string, error) {
	if token == "" {
		return "", nil
	}

	tokenName, tokenFilter, err := pagination.DecodeToken(token)
	if err != nil {
		return "", status.Error(codes.InvalidArgument, fmt.Sprintf("invalid PageToken: %v", err))
	}
	if filter != tokenFilter {
		return "", status.Error(codes.InvalidArgument, "filter does not match previous query")
	}
	return tokenName, nil
}
