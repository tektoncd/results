package main

import (
	"fmt"

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
