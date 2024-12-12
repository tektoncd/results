package fake

import (
	"context"
	"fmt"

	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

// ResultsClient is a fake implementation of the ResultsClient interface
type ResultsClient struct {
	// Map of result name to Result for GetResult and ListResults
	results map[string]*pb.Result

	// Map of record name to Record for GetRecord and ListRecords
	records map[string]*pb.Record
}

// NewResultsClient creates a new fake ResultsClient
func NewResultsClient(testResults []*pb.Result, testRecords []*pb.Record) *ResultsClient {
	r := &ResultsClient{
		results: make(map[string]*pb.Result),
		records: make(map[string]*pb.Record),
	}
	for _, result := range testResults {
		r.results[result.Name] = result
	}
	for _, record := range testRecords {
		r.records[record.Name] = record
	}
	return r
}

// GetResult implements ResultsClient.GetResult
func (c *ResultsClient) GetResult(_ context.Context, in *pb.GetResultRequest, _ ...grpc.CallOption) (*pb.Result, error) {
	result, exists := c.results[in.Name]
	if !exists {
		return nil, fmt.Errorf("result not found: %s", in.Name)
	}
	return result, nil
}

// ListResults implements ResultsClient.ListResults
func (c *ResultsClient) ListResults(_ context.Context, _ *pb.ListResultsRequest, _ ...grpc.CallOption) (*pb.ListResultsResponse, error) {
	results := make([]*pb.Result, 0, len(c.results))
	for _, result := range c.results {
		results = append(results, result)
	}

	return &pb.ListResultsResponse{
		Results: results,
	}, nil
}

// CreateResult is unimplemented
func (c *ResultsClient) CreateResult(_ context.Context, _ *pb.CreateResultRequest, _ ...grpc.CallOption) (*pb.Result, error) {
	return nil, fmt.Errorf("unimplemented")
}

// UpdateResult is unimplemented
func (c *ResultsClient) UpdateResult(_ context.Context, _ *pb.UpdateResultRequest, _ ...grpc.CallOption) (*pb.Result, error) {
	return nil, fmt.Errorf("unimplemented")
}

// DeleteResult is unimplemented
func (c *ResultsClient) DeleteResult(_ context.Context, _ *pb.DeleteResultRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, fmt.Errorf("unimplemented")
}

// CreateRecord is unimplemented
func (c *ResultsClient) CreateRecord(_ context.Context, _ *pb.CreateRecordRequest, _ ...grpc.CallOption) (*pb.Record, error) {
	return nil, fmt.Errorf("unimplemented")
}

// UpdateRecord is unimplemented
func (c *ResultsClient) UpdateRecord(_ context.Context, _ *pb.UpdateRecordRequest, _ ...grpc.CallOption) (*pb.Record, error) {
	return nil, fmt.Errorf("unimplemented")
}

// GetRecord is unimplemented
func (c *ResultsClient) GetRecord(_ context.Context, in *pb.GetRecordRequest, _ ...grpc.CallOption) (*pb.Record, error) {
	record, exists := c.records[in.Name]
	if !exists {
		return nil, fmt.Errorf("record not found: %s", in.Name)
	}
	return record, nil
}

// ListRecords is unimplemented
func (c *ResultsClient) ListRecords(_ context.Context, _ *pb.ListRecordsRequest, _ ...grpc.CallOption) (*pb.ListRecordsResponse, error) {
	records := make([]*pb.Record, 0, len(c.records))
	for _, record := range c.records {
		records = append(records, record)
	}
	return &pb.ListRecordsResponse{
		Records: records,
	}, nil
}

// DeleteRecord is unimplemented
func (c *ResultsClient) DeleteRecord(_ context.Context, _ *pb.DeleteRecordRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, fmt.Errorf("unimplemented")
}

// GetRecordListSummary is unimplemented
func (c *ResultsClient) GetRecordListSummary(_ context.Context, _ *pb.RecordListSummaryRequest, _ ...grpc.CallOption) (*pb.RecordListSummary, error) {
	return nil, fmt.Errorf("unimplemented")
}
