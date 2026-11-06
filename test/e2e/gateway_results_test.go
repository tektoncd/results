// Copyright 2026 The Tekton Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build e2e

package e2e

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"github.com/tektoncd/results/test/e2e/client"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"k8s.io/client-go/transport"
)

func deployedGatewayHTTP(t *testing.T, tokenFile string) *http.Client {
	t.Helper()
	cfg := &transport.Config{TLS: transport.TLSConfig{CAFile: envCfg.CertFile, ServerName: envCfg.ServerName}}
	if tokenFile != "" {
		cfg.BearerTokenFile = envCfg.TokenFile(tokenFile)
	}
	rt, err := transport.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Transport: rt, Timeout: 30 * time.Second}
}

func deployedGatewayFixture(t *testing.T, typ string, data []byte) (client.GRPCClient, *pb.Result, *pb.Record) {
	t.Helper()
	gc, _ := resultsClient(t, allNamespacesAdminAccessToken, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := gc.CreateResult(ctx, &pb.CreateResultRequest{Parent: defaultNamespace, Result: &pb.Result{Name: defaultNamespace + "/results/gateway-" + uuid.NewString()}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := gc.DeleteResult(ctx, &pb.DeleteResultRequest{Name: res.Name}); err != nil {
			t.Logf("fixture cleanup: %v", err)
		}
	})
	record, err := gc.CreateRecord(ctx, &pb.CreateRecordRequest{Parent: res.Name, Record: &pb.Record{Name: res.Name + "/records/gateway-" + uuid.NewString(), Data: &pb.Any{Type: typ, Value: data}}})
	if err != nil {
		t.Fatal(err)
	}
	return gc, res, record
}

func TestGatewayDeployedSummary(t *testing.T) {
	gc, res, _ := deployedGatewayFixture(t, "gateway.test/data", []byte(`{"fixture":true}`))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	native, err := gc.GetRecordListSummary(ctx, &pb.RecordListSummaryRequest{Parent: res.Name, Summary: "total"})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(envCfg.ServerAddress, "/")+"/apis/results.tekton.dev/v1alpha2/parents/"+res.Name+"/records/summary?summary=total", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := deployedGatewayHTTP(t, allNamespacesAdminAccessToken).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("REST summary: %d %s", resp.StatusCode, body)
	}
	got := &pb.RecordListSummary{}
	if err := protojson.Unmarshal(body, got); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(native, got) || len(got.Summary) != 1 || got.Summary[0].Fields["total"].GetNumberValue() != 1 {
		t.Fatalf("summary mismatch: native=%v REST=%v", native, got)
	}
}
