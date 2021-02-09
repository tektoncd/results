package main

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/types/known/anypb"

	_ "github.com/tektoncd/results/proto/pipeline/v1beta1/pipeline_go_proto"
)

func textproto(a *anypb.Any) (string, error) {
	m, err := a.UnmarshalNew()
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	return prototext.Format(m), nil
}

func parent(in string) string {
	s := strings.Split(in, "/")
	return strings.Join(s[:3], "/")
}
