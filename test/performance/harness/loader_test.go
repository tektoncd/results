/*
Copyright 2026 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"github.com/tektoncd/results/test/performance/generator"
	"github.com/tektoncd/results/test/performance/report"
)

func TestSummarizeSeedLoad(t *testing.T) {
	for _, tt := range []struct {
		name        string
		total       int
		failed      int
		failedCodes []string
	}{
		{name: "no instances"},
		{name: "all successful", total: 100},
		{name: "one percent", total: 100, failed: 1},
		{name: "49 percent", total: 100, failed: 49},
		{name: "50 percent", total: 100, failed: 50},
		{name: "51 percent", total: 100, failed: 51},
		{name: "one of three", total: 3, failed: 1},
		{name: "two of three", total: 3, failed: 2},
		{name: "all failed", total: 1, failed: 1},
		{name: "child write fails", total: 2, failed: 1, failedCodes: []string{"OK", "OK", "OK", "OK", "PermissionDenied"}},
		{name: "PipelineRun update fails", total: 2, failed: 1, failedCodes: []string{"OK", "OK", "Unavailable", "OK", "OK"}},
		{name: "multiple errors count once", total: 2, failed: 1, failedCodes: []string{"Unavailable", "PermissionDenied", "FailedPrecondition"}},
		{name: "errors concentrated in one instance", total: 3, failed: 1, failedCodes: strings.Fields(strings.Repeat("Unavailable ", 20))},
	} {
		t.Run(tt.name, func(t *testing.T) {
			log := &sendLog{}
			for i := 0; i < tt.total; i++ {
				// Vary successful call counts so each instance has equal weight
				// regardless of the number of children it contains.
				rpcCodes := make([]string, 4+i%10)
				for j := range rpcCodes {
					rpcCodes[j] = codes.OK.String()
				}
				if i < tt.failed {
					if tt.failedCodes != nil {
						rpcCodes = tt.failedCodes
					} else {
						rpcCodes[0] = codes.PermissionDenied.String()
					}
				}
				log.add(sendLogEntry{Index: i, Codes: rpcCodes})
			}
			want := &report.SeedLoadMetrics{TotalInstances: tt.total, FailedInstances: tt.failed}
			if tt.total > 0 {
				want.ErrorRatePercent = 100 * (float64(tt.failed) / float64(tt.total))
			}
			if diff := cmp.Diff(want, summarizeSeedLoad(log)); diff != "" {
				t.Errorf("seed-load summary mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

type seedResultsClient struct {
	pb.ResultsClient
	createResultErr  error
	createRecordErrs map[string]error
}

func (c seedResultsClient) CreateResult(_ context.Context, req *pb.CreateResultRequest, _ ...grpc.CallOption) (*pb.Result, error) {
	if c.createResultErr != nil {
		return nil, c.createResultErr
	}
	return req.GetResult(), nil
}

func (c seedResultsClient) CreateRecord(_ context.Context, req *pb.CreateRecordRequest, _ ...grpc.CallOption) (*pb.Record, error) {
	if err := c.createRecordErrs[req.GetRecord().GetName()]; err != nil {
		return nil, err
	}
	return req.GetRecord(), nil
}

func (seedResultsClient) UpdateRecord(_ context.Context, req *pb.UpdateRecordRequest, _ ...grpc.CallOption) (*pb.Record, error) {
	return req.GetRecord(), nil
}

func (seedResultsClient) UpdateResult(_ context.Context, req *pb.UpdateResultRequest, _ ...grpc.CallOption) (*pb.Result, error) {
	return req.GetResult(), nil
}

func TestLoadSeedChildErrorsReport(t *testing.T) {
	cfg := testRunConfig()
	cfg.Count = 3
	cfg.Concurrency = 2
	cfg.ChildMin = 2
	cfg.ChildMax = 2
	g, err := newGenerator(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	inst, err := g.At(0)
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []codes.Code{codes.PermissionDenied, codes.AlreadyExists} {
		t.Run(code.String(), func(t *testing.T) {
			cfg.OutputPath = filepath.Join(t.TempDir(), "load.json")
			client := seedResultsClient{createRecordErrs: map[string]error{
				inst.ChildRecords[0]: status.Error(code, "create child"),
			}}
			ctx := context.Background()
			m, def, log, err := loadSeed(ctx, client, cfg)
			if err != nil {
				t.Fatalf("loadSeed() = %v, want nonfatal write errors", err)
			}
			snap := m.Snapshot()
			if err := emitReport(ctx, cfg, "load", "grpc", reportExtra{
				datasetHash: def.ContentHash,
				seedLoad:    summarizeSeedLoad(log),
			}, snap); err != nil {
				t.Fatalf("emitReport() = %v", err)
			}
			data, err := os.ReadFile(cfg.OutputPath)
			if err != nil {
				t.Fatal(err)
			}
			var got report.Report
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("decoding load report: %v", err)
			}
			if got.Metrics.TotalErrors != 1 || got.Metrics.TotalOps != 18 {
				t.Errorf("reported API metrics = %d/%d errors, want 1/18", got.Metrics.TotalErrors, got.Metrics.TotalOps)
			}
			want := &report.SeedLoadMetrics{TotalInstances: 3, FailedInstances: 1, ErrorRatePercent: 100 * (1.0 / float64(cfg.Count))}
			if code == codes.AlreadyExists {
				want.FailedInstances = 0
				want.ErrorRatePercent = 0
			}
			if diff := cmp.Diff(want, got.Metrics.SeedLoad); diff != "" {
				t.Errorf("reported seed-load summary mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLoadReportSurvivesVerificationFailure(t *testing.T) {
	reportDir := t.TempDir()
	t.Setenv("TMPDIR", reportDir)
	server := grpc.NewServer()
	// Unimplemented writes fail nonfatally; ListRecords then fails verification.
	pb.RegisterResultsServer(server, &pb.UnimplementedResultsServer{})
	t.Cleanup(server.Stop)
	https := httptest.NewUnstartedServer(server)
	https.EnableHTTP2 = true
	https.StartTLS()
	t.Cleanup(https.Close)
	credentialsDir := t.TempDir()
	certPath := filepath.Join(credentialsDir, "ca.pem")
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: https.Certificate().Raw})
	if err := os.WriteFile(certPath, cert, 0o600); err != nil {
		t.Fatal(err)
	}
	tokenPath := filepath.Join(credentialsDir, "token")
	if err := os.WriteFile(tokenPath, []byte("test-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := newLoadCmd()
	cmd.SetArgs([]string{
		"--server-addr", https.URL, "--server-name", "127.0.0.1",
		"--cert", certPath, "--token", tokenPath,
		"--count", "2", "--child-min", "2", "--child-max", "2", "--verify",
	})
	if err := cmd.ExecuteContext(context.Background()); err == nil || !strings.Contains(err.Error(), "seed verification failed") {
		t.Fatalf("load --verify error = %v, want verification failure", err)
	}
	files, err := os.ReadDir(reportDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("default report directory contains %d entries, want one report", len(files))
	}
	data, err := os.ReadFile(filepath.Join(reportDir, files[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	var got report.Report
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decoding report after failed verification: %v", err)
	}
	want := &report.SeedLoadMetrics{TotalInstances: 2, FailedInstances: 2, ErrorRatePercent: 100}
	if diff := cmp.Diff(want, got.Metrics.SeedLoad); diff != "" {
		t.Errorf("seed-load summary after failed verification mismatch (-want +got):\n%s", diff)
	}
}

func TestLoadSeedSendLog(t *testing.T) {
	for _, code := range []codes.Code{codes.OK, codes.PermissionDenied} {
		t.Run(code.String(), func(t *testing.T) {
			cfg := testRunConfig()
			cfg.Count = 24
			cfg.Concurrency = 4
			cfg.OutputPath = filepath.Join(t.TempDir(), "nested", "load.json")
			client := seedResultsClient{createResultErr: status.Error(code, "create result")}
			m, def, log, err := loadSeed(context.Background(), client, cfg)
			if err != nil {
				t.Fatalf("loadSeed() = %v", err)
			}
			if err := writeSendLog(cfg.OutputPath, log); err != nil {
				t.Fatalf("writeSendLog() = %v", err)
			}
			data, err := os.ReadFile(filepath.Join(filepath.Dir(cfg.OutputPath), "load.sendlog.json"))
			if err != nil {
				t.Fatalf("reading load send log: %v", err)
			}
			var got []sendLogEntry
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("decoding load send log: %v", err)
			}
			want := make([]sendLogEntry, len(def.Instances))
			for i, inst := range def.Instances {
				// A seed load makes four top-level calls (create Result,
				// create/update Record, update Result), then creates children.
				rpcCodes := make([]string, 4+len(inst.ChildRecords))
				for j := range rpcCodes {
					rpcCodes[j] = codes.OK.String()
				}
				rpcCodes[0] = code.String()
				want[i] = sendLogEntry{
					Index:        i,
					UID:          inst.UID,
					ResultName:   inst.ResultName,
					RecordName:   inst.RecordName,
					ChildRecords: inst.ChildRecords,
					UpdateCount:  1,
					Codes:        rpcCodes,
				}
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("load send log mismatch (-want +got):\n%s", diff)
			}
			var wantErrors int64
			if code != codes.OK {
				wantErrors = int64(cfg.Count)
			}
			if got := m.Snapshot().TotalError; got != wantErrors {
				t.Errorf("load errors = %d, want %d", got, wantErrors)
			}
		})
	}
}

type recordListerFunc func(context.Context, *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error)

func (f recordListerFunc) ListRecords(ctx context.Context, req *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error) {
	return f(ctx, req)
}

func (recordListerFunc) ListResults(context.Context, *pb.ListResultsRequest) (*pb.ListResultsResponse, error) {
	return nil, fmt.Errorf("unexpected ListResults call")
}

func TestVerifySeed(t *testing.T) {
	def := &generator.DatasetDefinition{
		Count: 3,
		PerNamespace: map[string]generator.NamespaceCounts{
			"ns-0": {PipelineRuns: 1, TaskRuns: 2},
			"ns-1": {PipelineRuns: 2, TaskRuns: 3},
		},
	}
	tests := []struct {
		name    string
		counts  map[string]generator.NamespaceCounts
		wantErr bool
	}{
		{
			name:   "matching namespace counts",
			counts: def.PerNamespace,
		},
		{
			name: "all records in one namespace with matching totals",
			counts: map[string]generator.NamespaceCounts{
				"ns-0": {PipelineRuns: 3, TaskRuns: 5},
			},
			wantErr: true,
		},
		{
			name: "swapped PipelineRun counts with matching totals",
			counts: map[string]generator.NamespaceCounts{
				"ns-0": {PipelineRuns: 2, TaskRuns: 2},
				"ns-1": {PipelineRuns: 1, TaskRuns: 3},
			},
			wantErr: true,
		},
		{
			name: "swapped TaskRun counts with matching totals",
			counts: map[string]generator.NamespaceCounts{
				"ns-0": {PipelineRuns: 1, TaskRuns: 3},
				"ns-1": {PipelineRuns: 2, TaskRuns: 2},
			},
			wantErr: true,
		},
		{
			name: "missing namespace",
			counts: map[string]generator.NamespaceCounts{
				"ns-0": {PipelineRuns: 1, TaskRuns: 2},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := recordListerFunc(func(_ context.Context, req *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error) {
				ns := strings.TrimSuffix(req.GetParent(), "/results/-")
				counts := tt.counts[ns]
				// Preserve wildcard semantics so the regressions expose the old
				// verifier accepting matching totals in the wrong namespaces.
				if ns == "-" {
					for _, nc := range tt.counts {
						counts.PipelineRuns += nc.PipelineRuns
						counts.TaskRuns += nc.TaskRuns
					}
				}
				var count int
				switch req.GetFilter() {
				case "data_type == PIPELINE_RUN":
					count = counts.PipelineRuns
				case "data_type == TASK_RUN":
					count = counts.TaskRuns
				default:
					t.Fatalf("unexpected filter %q", req.GetFilter())
				}
				return &pb.ListRecordsResponse{Records: make([]*pb.Record, count)}, nil
			})
			err := verifySeed(context.Background(), l, def)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "row-count mismatch in namespace \"ns-") {
					t.Fatalf("verifySeed() = %v, want a namespace-specific count mismatch", err)
				}
			} else if err != nil {
				t.Fatalf("verifySeed() = %v, want nil", err)
			}
		})
	}
}

func TestVerifySeedPagination(t *testing.T) {
	def := &generator.DatasetDefinition{
		Count: 1001,
		PerNamespace: map[string]generator.NamespaceCounts{
			"ns-0": {PipelineRuns: 1001, TaskRuns: 2001},
		},
	}
	l := recordListerFunc(func(_ context.Context, req *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error) {
		if req.GetParent() != "ns-0/results/-" {
			t.Fatalf("parent = %q, want ns-0/results/-", req.GetParent())
		}
		count := def.PerNamespace["ns-0"].PipelineRuns
		if req.GetFilter() == "data_type == TASK_RUN" {
			count = def.PerNamespace["ns-0"].TaskRuns
		}
		offset := 0
		if req.GetPageToken() != "" {
			var err error
			offset, err = strconv.Atoi(req.GetPageToken())
			if err != nil {
				t.Fatalf("invalid page token: %v", err)
			}
		}
		n := min(count-offset, int(req.GetPageSize()))
		resp := &pb.ListRecordsResponse{Records: make([]*pb.Record, n)}
		if offset+n < count {
			resp.NextPageToken = strconv.Itoa(offset + n)
		}
		return resp, nil
	})
	if err := verifySeed(context.Background(), l, def); err != nil {
		t.Fatalf("verifySeed() = %v, want nil", err)
	}
}

func TestVerifySeedListError(t *testing.T) {
	def := &generator.DatasetDefinition{
		Count: 1,
		PerNamespace: map[string]generator.NamespaceCounts{
			"ns-0": {PipelineRuns: 1, TaskRuns: 2},
		},
	}
	wantErr := errors.New("list failed")
	for _, kind := range []string{"PipelineRun", "TaskRun"} {
		t.Run(kind, func(t *testing.T) {
			l := recordListerFunc(func(_ context.Context, req *pb.ListRecordsRequest) (*pb.ListRecordsResponse, error) {
				if kind == "PipelineRun" || req.GetFilter() == "data_type == TASK_RUN" {
					return nil, wantErr
				}
				return &pb.ListRecordsResponse{Records: []*pb.Record{{}}}, nil
			})
			err := verifySeed(context.Background(), l, def)
			if !errors.Is(err, wantErr) {
				t.Fatalf("verifySeed() = %v, want wrapped %v", err, wantErr)
			}
			if !strings.Contains(err.Error(), kind) || !strings.Contains(err.Error(), `namespace "ns-0"`) {
				t.Errorf("verifySeed() = %v, want record type and namespace in error", err)
			}
		})
	}
}
