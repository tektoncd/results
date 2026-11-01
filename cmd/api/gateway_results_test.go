package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	model "github.com/tektoncd/results/pkg/api/server/db"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

func gatewayJSON(t *testing.T, w *httptest.ResponseRecorder, want int) map[string]any {
	t.Helper()
	if w.Code != want {
		t.Fatalf("status=%d body=%s, want %d", w.Code, w.Body, want)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestGatewayResultsAndRecords(t *testing.T) {
	for _, baseline := range []bool{true, false} {
		t.Run(fmt.Sprintf("baseline=%v", baseline), func(t *testing.T) {
			g := newGatewayFixture(t, baseline, nil, nil)
			resultPath := resultsURL + "/one"
			recordPath := resultPath + "/records/entry"
			r := gatewayJSON(t, gatewayRequest(t, g.handler, "POST", resultsURL, []byte(`{"name":"foo/results/one","unknown":"discarded"}`), nil), 200)
			if r["name"] != "foo/results/one" || r["created_time"] == nil || r["etag"] == nil {
				t.Fatalf("result contract: %v", r)
			}
			gatewayJSON(t, gatewayRequest(t, g.handler, "GET", resultPath, nil, nil), 200)
			gatewayJSON(t, gatewayRequest(t, g.handler, "PATCH", resultPath+"?name=foo/results/one&update_mask=annotations", []byte(`{"name":"foo/results/one","annotations":{"updated":"yes"}}`), nil), 200)
			data := base64.StdEncoding.EncodeToString([]byte(`{"a":{"b":"selected"},"other":true}`))
			body := []byte(fmt.Sprintf(`{"name":"foo/results/one/records/entry","data":{"type":"example.com/test","value":%q}}`, data))
			gatewayJSON(t, gatewayRequest(t, g.handler, "POST", resultPath+"/records", body, nil), 200)
			gatewayJSON(t, gatewayRequest(t, g.handler, "GET", recordPath, nil, nil), 200)
			gatewayJSON(t, gatewayRequest(t, g.handler, "PATCH", recordPath+"?name=foo/results/one/records/entry&update_mask=data", []byte(fmt.Sprintf(`{"name":"foo/results/one/records/entry","data":{"type":"example.com/test","value":%q}}`, data)), nil), 200)
			list := gatewayJSON(t, gatewayRequest(t, g.handler, "GET", resultPath+"/records", nil, nil), 200)
			if len(list["records"].([]any)) != 1 {
				t.Fatalf("records: %v", list)
			}
			g.partial.Store(true)
			selected := gatewayJSON(t, gatewayRequest(t, g.handler, "GET", recordPath+"?fields=name,data.value.a.b", nil, nil), 200)
			if selected["name"] == nil || selected["etag"] != nil || selected["id"] != nil {
				t.Fatalf("partial response: %v", selected)
			}
			value := selected["data"].(map[string]any)["value"].(string)
			decoded, err := base64.StdEncoding.DecodeString(value)
			if err != nil || string(decoded) != `{"a":{"b":"selected"}}` {
				t.Fatalf("nested partial JSON %s: %v", decoded, err)
			}
			g.partial.Store(false)
			full := gatewayJSON(t, gatewayRequest(t, g.handler, "GET", recordPath+"?fields=name", nil, nil), 200)
			if full["etag"] == nil {
				t.Fatal("disabled partial responses lost fields")
			}
			gatewayJSON(t, gatewayRequest(t, g.handler, "DELETE", recordPath, nil, nil), 200)
			gatewayJSON(t, gatewayRequest(t, g.handler, "GET", recordPath, nil, nil), 404)
			gatewayJSON(t, gatewayRequest(t, g.handler, "DELETE", resultPath, nil, nil), 200)
		})
	}
}

func TestGatewayPagination(t *testing.T) {
	for _, baseline := range []bool{true, false} {
		g := newGatewayFixture(t, baseline, nil, nil)
		var targetID string
		for i := range 11 {
			created := gatewayJSON(t, gatewayRequest(t, g.handler, "POST", resultsURL, []byte(fmt.Sprintf(`{"name":"foo/results/r%d","annotations":{"index":"%d"}}`, i, i)), nil), 200)
			if i == 1 {
				targetID = created["uid"].(string)
			}
		}
		seen := make(map[string]bool)
		path := resultsURL + "?page_size=5&order_by=create_time"
		for {
			body := gatewayJSON(t, gatewayRequest(t, g.handler, "GET", path, nil, nil), 200)
			for _, result := range body["results"].([]any) {
				name := result.(map[string]any)["name"].(string)
				if seen[name] {
					t.Fatalf("duplicate result: %s", name)
				}
				seen[name] = true
			}
			token, _ := body["next_page_token"].(string)
			if token == "" {
				break
			}
			path = resultsURL + "?page_size=5&order_by=create_time&page_token=" + url.QueryEscape(token)
		}
		if len(seen) != 11 {
			t.Errorf("baseline=%v: got %d results", baseline, len(seen))
		}
		filter := url.QueryEscape(fmt.Sprintf(`uid == %q`, targetID))
		body := gatewayJSON(t, gatewayRequest(t, g.handler, "GET", resultsURL+"?filter="+filter, nil, nil), 200)
		if entries, _ := body["results"].([]any); len(entries) != 1 {
			t.Fatalf("filtered results: %v", body)
		}
	}
}

func TestGatewayErrorsMatchBaseline(t *testing.T) {
	baseline := newGatewayFixture(t, true, nil, nil)
	direct := newGatewayFixture(t, false, nil, nil)
	for _, tc := range []struct {
		method, path, body string
	}{
		{"POST", resultsURL, "{"},
		{"POST", resultsURL, `{}`},
		{"GET", resultsURL + "/missing", ""},
		{"GET", resultsURL + "?page_token=invalid", ""},
		{"GET", resultsURL + "/one/records/summary?summary=total", ""},
		{"GET", resultsURL + "?filter=invalid%20syntax", ""},
		{"PATCH", resultsURL + "/missing", `{"annotations":{"a":"b"},"etag":"stale"}`},
	} {
		old := gatewayRequest(t, baseline.handler, tc.method, tc.path, []byte(tc.body), nil)
		got := gatewayRequest(t, direct.handler, tc.method, tc.path, []byte(tc.body), nil)
		oldBody := gatewayJSON(t, old, old.Code)
		gotBody := gatewayJSON(t, got, old.Code)
		if oldBody["code"] != gotBody["code"] || oldBody["message"] != gotBody["message"] || old.Header().Get("Content-Type") != got.Header().Get("Content-Type") {
			t.Errorf("%s %s: baseline=%v direct=%v", tc.method, tc.path, oldBody, gotBody)
		}
	}
}

func TestGatewayRequestSize(t *testing.T) {
	for _, baseline := range []bool{true, false} {
		g := newGatewayFixture(t, baseline, nil, nil)
		gatewayJSON(t, gatewayRequest(t, g.handler, "POST", resultsURL, []byte(`{"name":"foo/results/large"}`), nil), 200)
		// Base64 expands the JSON body beyond 4 MiB; the protobuf still fits.
		value := []byte(`{"large":"` + strings.Repeat("a", 3*1024*1024+256) + `"}`)
		body := []byte(fmt.Sprintf(`{"name":"foo/results/large/records/r","data":{"type":"test","value":%q}}`, base64.StdEncoding.EncodeToString(value)))
		if len(body) <= 4*1024*1024 {
			t.Fatal("fixture must exceed the JSON-body size")
		}
		gatewayJSON(t, gatewayRequest(t, g.handler, "POST", resultsURL+"/large/records", body, nil), 200)
		oversize := []byte(fmt.Sprintf(`{"name":"foo/results/oversize","annotations":{"large":%q}}`, strings.Repeat("a", 4*1024*1024)))
		gatewayJSON(t, gatewayRequest(t, g.handler, "POST", resultsURL, oversize, nil), 429)
	}
}

type blockingChecker struct{ calls atomic.Int64 }

func (c *blockingChecker) Check(ctx context.Context, _, _, _ string) error {
	c.calls.Add(1)
	<-ctx.Done()
	// Match RBAC's error when an upstream review fails; transport cancellation must win.
	return status.Error(codes.Unauthenticated, "permission denied")
}

func TestGatewayCancellation(t *testing.T) {
	for _, baseline := range []bool{true, false} {
		for _, during := range []bool{false, true} {
			checker := &blockingChecker{}
			g := newGatewayFixture(t, baseline, nil, checker)
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			if during {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 30*time.Millisecond)
			}
			req := httptest.NewRequest("GET", resultsURL, nil).WithContext(ctx)
			w := httptest.NewRecorder()
			g.handler.ServeHTTP(w, req)
			cancel()
			if w.Code != 504 {
				t.Errorf("baseline=%v during=%v: %d %s", baseline, during, w.Code, w.Body)
			}
			if !during && checker.calls.Load() != 0 {
				t.Errorf("already expired request invoked auth %d times", checker.calls.Load())
			}
		}
	}
}

func TestGatewayDatabaseCancellation(t *testing.T) {
	for _, tc := range []struct {
		name, method, path string
	}{
		{"GetResult", "GET", resultsURL + "/missing"},
		{"DeleteRecord", "DELETE", resultsURL + "/missing/records/missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGatewayFixture(t, false, nil, nil)
			started := make(chan struct{}, 1)
			release := make(chan struct{})
			if err := g.db.Callback().Query().Before("gorm:query").Register("test:wait_for_cancellation", func(tx *gorm.DB) {
				select {
				case started <- struct{}{}:
				default:
				}
				select {
				case <-tx.Statement.Context.Done():
					tx.AddError(tx.Statement.Context.Err())
				case <-release:
				}
			}); err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req := httptest.NewRequest(tc.method, tc.path, nil).WithContext(ctx)
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				w := httptest.NewRecorder()
				g.handler.ServeHTTP(w, req)
				done <- w
			}()

			select {
			case <-started:
			case <-time.After(3 * time.Second):
				cancel()
				close(release)
				t.Fatal("database lookup did not start")
			}
			cancel()
			select {
			case w := <-done:
				gatewayJSON(t, w, 499)
			case <-time.After(3 * time.Second):
				close(release)
				<-done
				t.Fatal("REST request remained blocked after cancellation during database lookup")
			}
		})
	}
}

func TestGatewayResponseSize(t *testing.T) {
	for _, baseline := range []bool{true, false} {
		g := newGatewayFixture(t, baseline, nil, nil)
		// Seed storage through the service fixture to exercise response limits independently
		// of the smaller request limit. Production clients still have the request ceiling.
		large := strings.Repeat("a", restMaxResponseSize)
		r, err := g.server.CreateResult(context.Background(), &pb.CreateResultRequest{Parent: "foo", Result: &pb.Result{Name: "foo/results/large", Annotations: map[string]string{"large": large}}})
		if err != nil {
			t.Fatal(err)
		}
		path := resultsURL + "/large"
		gatewayJSON(t, gatewayRequest(t, g.handler, "GET", path, nil, nil), 429)
		g.partial.Store(true)
		gatewayJSON(t, gatewayRequest(t, g.handler, "GET", path+"?fields=name", nil, nil), 200)
		g.partial.Store(false)
		stored, err := g.server.GetResult(context.Background(), &pb.GetResultRequest{Name: r.Name})
		if err != nil {
			t.Fatal(err)
		}
		large = large[:len(large)-(proto.Size(stored)-restMaxResponseSize)]
		if err := g.db.Model(&model.Result{}).Where("parent = ? AND name = ?", "foo", "large").Update("annotations", model.Annotations{"large": large}).Error; err != nil {
			t.Fatal(err)
		}
		exact, err := g.server.GetResult(context.Background(), &pb.GetResultRequest{Name: r.Name})
		if err != nil {
			t.Fatal(err)
		}
		if proto.Size(exact) != restMaxResponseSize {
			t.Fatalf("boundary fixture size=%d", proto.Size(exact))
		}
		w := gatewayRequest(t, g.handler, "GET", path, nil, nil)
		if w.Code != 200 {
			t.Fatalf("baseline=%v exact response size status=%d", baseline, w.Code)
		}
	}
}
