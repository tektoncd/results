/*
Copyright 2023 The Tekton Authors

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
package log

import (
	"bytes"
	"context"
	"flag"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-replayers/httpreplay"
	hrgoog "github.com/google/go-replayers/httpreplay/google"
	server "github.com/tektoncd/results/pkg/api/server/config"
	"gocloud.dev/gcp"
	"google.golang.org/api/option"
)

var (
	// Record is true if the tests are being run in "record" mode.
	// If replay file needs to be generated then please give -record during run of tests
	Record = flag.Bool("record", false,
		"whether to run tests against cloud resources and record the interactions")
	gcsTestLogData    = "foo-bar-log"
	gcsTestBucketName = "tekton-releases-test-results"
	gcsTestKey        = "foo/bar/log"
)

// This function is to create a replay or record http client depending
// upon whether a -record flag was passed durin testing
func NewRecordReplayClient(t *testing.T, modReq func(r *httpreplay.Recorder), port int) (*http.Client, func()) {
	httpreplay.DebugHeaders()
	path := filepath.Join("testdata", t.Name()+".replay")
	if *Record {
		t.Logf("Recording GCS API responses into replay file %s", path)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}

		state := time.Now()
		b, _ := state.MarshalBinary()
		rec, err := httpreplay.NewRecorderWithOpts(path, httpreplay.RecorderInitial(b), httpreplay.RecorderPort(port))
		if err != nil {
			t.Fatal(err)
		}

		modReq(rec)

		cleanup := func() {
			if err := rec.Close(); err != nil {
				t.Fatal(err)
			}
			t.Log("saved the record")
		}

		return rec.Client(), cleanup
	}

	t.Logf("Replaying GCS API responses from replay file %s", path)
	rep, err := httpreplay.NewReplayerWithOpts(path, httpreplay.ReplayerPort(port))
	if err != nil {
		t.Fatal(err)
	}

	recState := new(time.Time)
	if err := recState.UnmarshalBinary(rep.Initial()); err != nil {
		t.Fatal(err)
	}

	cleanup := func() {
		rep.Close()
	}

	return rep.Client(), cleanup
}

func NewTestGCPClient(ctx context.Context, port int, t *testing.T) (client *gcp.HTTPClient, done func()) {
	c, cleanup := NewRecordReplayClient(t, func(r *httpreplay.Recorder) {
		r.ClearQueryParams("Expires")
		r.ClearQueryParams("Signature")
		r.ClearHeaders("Expires")
		r.ClearHeaders("Signature")
		r.ClearHeaders("X-Goog-Gcs-Idempotency-Token")
		r.ClearHeaders("User-Agent")
	}, port)
	if *Record {
		creds, err := gcp.DefaultCredentials(ctx)
		if err != nil {
			t.Fatalf("failed to get default credentials: %v", err)
		}
		c, err = hrgoog.RecordClient(ctx, c, option.WithTokenSource(gcp.CredentialsTokenSource(creds)))
		if err != nil {
			t.Fatal(err)
		}
	}
	return &gcp.HTTPClient{Client: *c}, cleanup
}

func TestGCSReadFrom(t *testing.T) {
	ctx := context.Background()
	gcs := &gcsStream{
		ctx: ctx,
		config: &server.Config{
			GCS_BUCKET_NAME: gcsTestBucketName,
		},
		key: gcsTestKey,
	}
	client, done := NewTestGCPClient(ctx, 8080, t)
	defer done()
	gcs.client = client
	reader := strings.NewReader(gcsTestLogData)
	_, err := gcs.ReadFrom(reader)
	if err != nil {
		t.Fatalf("failed to write to gcs: %v", err)
	}
}

func TestGCSWriteTo(t *testing.T) {
	ctx := context.Background()
	gcs := &gcsStream{
		ctx: ctx,
		config: &server.Config{
			GCS_BUCKET_NAME: gcsTestBucketName,
		},
		key: gcsTestKey,
	}
	client, done := NewTestGCPClient(ctx, 8081, t)
	defer done()
	gcs.client = client
	var w bytes.Buffer
	_, err := gcs.WriteTo(io.Writer(&w))
	if err != nil {
		t.Fatalf("failed to read from gcs: %v", err)
	}
	if w.String() != gcsTestLogData {
		t.Fatalf("value mismatch, expected: %s, got: %v", gcsTestLogData, w.String())
	}
}

func TestGCSDelete(t *testing.T) {
	ctx := context.Background()
	gcs := &gcsStream{
		ctx: ctx,
		config: &server.Config{
			GCS_BUCKET_NAME: gcsTestBucketName,
		},
		key: gcsTestKey,
	}
	client, done := NewTestGCPClient(ctx, 8083, t)
	defer done()
	gcs.client = client
	err := gcs.Delete()
	if err != nil {
		t.Fatalf("failed to delete key: %s", gcs.key)
	}
}
