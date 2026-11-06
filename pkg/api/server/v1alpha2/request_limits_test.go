package server

import (
	"context"
	"strings"
	"testing"

	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestRESTRequestLimit(t *testing.T) {
	for _, delta := range []int{-1, 0, 1} {
		req := &pb.GetResultRequest{Name: strings.Repeat("a", restMaxRequestSize-5+delta)}
		if proto.Size(req) != restMaxRequestSize+delta {
			t.Fatal("incorrect boundary fixture")
		}
		ctx := context.WithValue(context.Background(), directRESTRequestKey{}, &directRESTRequest{})
		err := checkRESTRequestSize(ctx, req)
		want := codes.OK
		if delta > 0 {
			want = codes.ResourceExhausted
		}
		if status.Code(err) != want {
			t.Errorf("size=%d: %v, want %v", proto.Size(req), err, want)
		}
		if err := checkRESTRequestSize(context.Background(), req); err != nil {
			t.Errorf("native request acquired a REST limit: %v", err)
		}
	}
}

func TestRESTGuardAtEveryEntry(t *testing.T) {
	big := strings.Repeat("a", restMaxRequestSize)
	s := &Server{}
	calls := []struct {
		name string
		call func(context.Context) error
	}{
		{"CreateResult", func(ctx context.Context) error {
			_, err := s.CreateResult(ctx, &pb.CreateResultRequest{Parent: big})
			return err
		}},
		{"GetResult", func(ctx context.Context) error {
			_, err := s.GetResult(ctx, &pb.GetResultRequest{Name: big})
			return err
		}},
		{"UpdateResult", func(ctx context.Context) error {
			_, err := s.UpdateResult(ctx, &pb.UpdateResultRequest{Result: &pb.Result{Name: big}})
			return err
		}},
		{"DeleteResult", func(ctx context.Context) error {
			_, err := s.DeleteResult(ctx, &pb.DeleteResultRequest{Name: big})
			return err
		}},
		{"ListResults", func(ctx context.Context) error {
			_, err := s.ListResults(ctx, &pb.ListResultsRequest{Filter: big})
			return err
		}},
		{"CreateRecord", func(ctx context.Context) error {
			_, err := s.CreateRecord(ctx, &pb.CreateRecordRequest{Parent: big})
			return err
		}},
		{"GetRecord", func(ctx context.Context) error {
			_, err := s.GetRecord(ctx, &pb.GetRecordRequest{Name: big})
			return err
		}},
		{"UpdateRecord", func(ctx context.Context) error {
			_, err := s.UpdateRecord(ctx, &pb.UpdateRecordRequest{Record: &pb.Record{Name: big}})
			return err
		}},
		{"DeleteRecord", func(ctx context.Context) error {
			_, err := s.DeleteRecord(ctx, &pb.DeleteRecordRequest{Name: big})
			return err
		}},
		{"ListRecords", func(ctx context.Context) error {
			_, err := s.ListRecords(ctx, &pb.ListRecordsRequest{PageToken: big})
			return err
		}},
		{"Summary", func(ctx context.Context) error {
			_, err := s.GetRecordListSummary(ctx, &pb.RecordListSummaryRequest{Parent: big})
			return err
		}},
	}
	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), directRESTRequestKey{}, &directRESTRequest{})
			// An empty Server will panic if auth or storage is reached.
			if err := tc.call(ctx); status.Code(err) != codes.ResourceExhausted {
				t.Fatalf("entry guard: %v", err)
			}
			ctx, cancel := context.WithCancel(ctx)
			cancel()
			if err := tc.call(ctx); status.Code(err) != codes.Canceled {
				t.Fatalf("canceled entry: %v", err)
			}
		})
	}
}

func TestRESTErrorPreservesBindingErrors(t *testing.T) {
	ctx := context.WithValue(context.Background(), directRESTRequestKey{}, &directRESTRequest{})
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	err := status.Error(codes.InvalidArgument, "invalid JSON")
	if RESTError(ctx, err) != err {
		t.Fatal("binding error precedence changed")
	}
	_ = checkRESTRequestSize(ctx, &pb.GetResultRequest{})
	if got := RESTError(ctx, status.Error(codes.Unauthenticated, "review interrupted")); status.Code(got) != codes.Canceled {
		t.Fatalf("dispatched cancellation: %v", got)
	}
	if RESTError(context.Background(), err) != err {
		t.Fatal("native error changed")
	}
}
