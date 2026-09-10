package main

import (
	"strings"

	_ "github.com/tektoncd/results/proto/pipeline/v1/pipeline_go_proto"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

func textproto(a *pb.Any) string {
	return a.String()
}

func parent(in string) string {
	s := strings.Split(in, "/")
	return strings.Join(s[:3], "/")
}
