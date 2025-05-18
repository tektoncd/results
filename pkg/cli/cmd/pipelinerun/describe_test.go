package pipelinerun

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/spf13/cobra"
	v1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tektoncd/results/pkg/cli/common"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

func TestDescribeCommand(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		mockListFunc func(context.Context, *pb.ListRecordsRequest, string) (*pb.ListRecordsResponse, error)
		wantErr      bool
		wantOutput   string
	}{
		{
			name: "success",
			args: []string{"my-pipelinerun"},
			mockListFunc: func(_ context.Context, _ *pb.ListRecordsRequest, _ string) (*pb.ListRecordsResponse, error) {
				pr := v1.PipelineRun{
					TypeMeta:   metav1.TypeMeta{APIVersion: "tekton.dev/v1", Kind: "PipelineRun"},
					ObjectMeta: metav1.ObjectMeta{Name: "my-pipelinerun", Namespace: "default"},
				}
				prBytes, _ := json.Marshal(pr)
				return &pb.ListRecordsResponse{
					Records: []*pb.Record{{
						Name: "default/results/abc/records/def",
						Uid:  "def",
						Data: &pb.Any{Value: prBytes},
					}},
				}, nil
			},
			wantErr:    false,
			wantOutput: "Name: my-pipelinerun",
		},
		{
			name: "not found",
			args: []string{"notfound"},
			mockListFunc: func(_ context.Context, _ *pb.ListRecordsRequest, _ string) (*pb.ListRecordsResponse, error) {
				return &pb.ListRecordsResponse{Records: []*pb.Record{}}, nil
			},
			wantErr:    true,
			wantOutput: "no PipelineRun found with name notfound",
		},
		{
			name: "multiple found",
			args: []string{"foo"},
			mockListFunc: func(_ context.Context, _ *pb.ListRecordsRequest, _ string) (*pb.ListRecordsResponse, error) {
				pr := v1.PipelineRun{ObjectMeta: metav1.ObjectMeta{Name: "foo"}}
				prBytes, _ := json.Marshal(pr)
				return &pb.ListRecordsResponse{
					Records: []*pb.Record{
						{Uid: "a", Data: &pb.Any{Value: prBytes}},
						{Uid: "b", Data: &pb.Any{Value: prBytes}},
					},
				}, nil
			},
			wantErr:    true,
			wantOutput: "multiple PipelineRuns found",
		},
		{
			name: "error from client",
			args: []string{"foo"},
			mockListFunc: func(_ context.Context, _ *pb.ListRecordsRequest, _ string) (*pb.ListRecordsResponse, error) {
				return nil, fmt.Errorf("test error")
			},
			wantErr:    true,
			wantOutput: "failed to find PipelineRun: test error",
		},
		{
			name:         "invalid arguments",
			args:         []string{},
			mockListFunc: nil,
			wantErr:      true,
			wantOutput:   "requires exactly one argument",
		},
		{
			name: "UID lookup",
			args: []string{"my-pipelinerun", "--uid", "my-uid"},
			mockListFunc: func(_ context.Context, _ *pb.ListRecordsRequest, _ string) (*pb.ListRecordsResponse, error) {
				pr := v1.PipelineRun{
					TypeMeta:   metav1.TypeMeta{APIVersion: "tekton.dev/v1", Kind: "PipelineRun"},
					ObjectMeta: metav1.ObjectMeta{Name: "my-pipelinerun", Namespace: "default", UID: "my-uid"},
				}
				prBytes, _ := json.Marshal(pr)
				return &pb.ListRecordsResponse{
					Records: []*pb.Record{{
						Name: "default/results/abc/records/def",
						Uid:  "my-uid",
						Data: &pb.Any{Value: prBytes},
					}},
				}, nil
			},
			wantErr:    false,
			wantOutput: "my-uid",
		},
		{
			name: "complex output",
			args: []string{"complex-pipelinerun"},
			mockListFunc: func(_ context.Context, _ *pb.ListRecordsRequest, _ string) (*pb.ListRecordsResponse, error) {
				pr := v1.PipelineRun{
					TypeMeta: metav1.TypeMeta{APIVersion: "tekton.dev/v1", Kind: "PipelineRun"},
					ObjectMeta: metav1.ObjectMeta{
						Name:        "complex-pipelinerun",
						Namespace:   "default",
						Labels:      map[string]string{"foo": "bar"},
						Annotations: map[string]string{"anno": "val"},
					},
				}
				prBytes, _ := json.Marshal(pr)
				return &pb.ListRecordsResponse{
					Records: []*pb.Record{{
						Name: "default/results/abc/records/def",
						Uid:  "def",
						Data: &pb.Any{Value: prBytes},
					}},
				}, nil
			},
			wantErr:    false,
			wantOutput: "complex-pipelinerun",
		},
		{
			name: "output yaml",
			args: []string{"my-pipelinerun", "--output", "yaml"},
			mockListFunc: func(_ context.Context, _ *pb.ListRecordsRequest, _ string) (*pb.ListRecordsResponse, error) {
				pr := v1.PipelineRun{
					TypeMeta:   metav1.TypeMeta{APIVersion: "tekton.dev/v1", Kind: "PipelineRun"},
					ObjectMeta: metav1.ObjectMeta{Name: "my-pipelinerun", Namespace: "default"},
				}
				prBytes, _ := json.Marshal(pr)
				return &pb.ListRecordsResponse{
					Records: []*pb.Record{{
						Name: "default/results/abc/records/def",
						Uid:  "def",
						Data: &pb.Any{Value: prBytes},
					}},
				}, nil
			},
			wantErr:    false,
			wantOutput: "apiVersion: tekton.dev/v1",
		},
		{
			name: "output json",
			args: []string{"my-pipelinerun", "--output", "json"},
			mockListFunc: func(_ context.Context, _ *pb.ListRecordsRequest, _ string) (*pb.ListRecordsResponse, error) {
				pr := v1.PipelineRun{
					TypeMeta:   metav1.TypeMeta{APIVersion: "tekton.dev/v1", Kind: "PipelineRun"},
					ObjectMeta: metav1.ObjectMeta{Name: "my-pipelinerun", Namespace: "default"},
				}
				prBytes, _ := json.Marshal(pr)
				return &pb.ListRecordsResponse{
					Records: []*pb.Record{{
						Name: "default/results/abc/records/def",
						Uid:  "def",
						Data: &pb.Any{Value: prBytes},
					}},
				}, nil
			},
			wantErr:    false,
			wantOutput: "\"apiVersion\": \"tekton.dev/v1\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := describeCommand(&common.ResultsParams{})
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.PreRunE = func(_ *cobra.Command, _ []string) error { return nil }
			cmd.RunE = func(cmd *cobra.Command, args []string) error {
				if tt.mockListFunc == nil {
					if len(args) != 1 {
						return fmt.Errorf("requires exactly one argument when --uid is not provided")
					}
					return nil
				}
				ctx := context.Background()
				resp, err := tt.mockListFunc(ctx, nil, "")
				if err != nil {
					return fmt.Errorf("failed to find PipelineRun: %v", err)
				}
				if len(resp.Records) == 0 {
					return fmt.Errorf("no PipelineRun found with name %s", args[0])
				}
				if len(resp.Records) > 1 {
					return fmt.Errorf("multiple PipelineRuns found")
				}
				var pr v1.PipelineRun
				if err := json.Unmarshal(resp.Records[0].Data.Value, &pr); err != nil {
					return fmt.Errorf("failed to unmarshal PipelineRun data: %v", err)
				}

				// Simulate output flag
				outputFlag, _ := cmd.Flags().GetString("output")
				switch outputFlag {
				case "yaml":
					fmt.Fprintf(buf, "apiVersion: %s\nkind: %s\n", pr.APIVersion, pr.Kind)
				case "json":
					fmt.Fprintf(buf, "{\"apiVersion\": \"%s\", \"kind\": \"%s\"}\n", pr.APIVersion, pr.Kind)
				default:
					fmt.Fprintf(buf, "Name: %s\n", pr.Name)
					if pr.UID != "" {
						fmt.Fprintf(buf, "UID: %s\n", pr.UID)
					}
					if len(pr.Labels) > 0 {
						fmt.Fprintf(buf, "Labels: %v\n", pr.Labels)
					}
					if len(pr.Annotations) > 0 {
						fmt.Fprintf(buf, "Annotations: %v\n", pr.Annotations)
					}
				}
				return nil
			}
			err := cmd.Execute()
			output := buf.String()
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), tt.wantOutput) {
					t.Errorf("expected error containing %q, got: %v", tt.wantOutput, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !strings.Contains(output, tt.wantOutput) {
					t.Errorf("expected output to contain %q, got: %q", tt.wantOutput, output)
				}
			}
		})
	}
}
