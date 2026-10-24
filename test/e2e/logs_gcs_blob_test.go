//go:build e2e && gcs_blob

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

// Package e2e contains GCS blob log backend e2e tests.
//
// These tests exercise the v1alpha3 log plugin with a GCS backend (gs:// URL)
// via the fake-gcs-server emulator.
//
// Vector is not used for the GCS leg. Its gcp_cloud_storage sink speaks the
// GCS XML API, which fake-gcs-server does not implement. Instead, test data
// is seeded directly via the JSON API (see 02-logs-setup-gcs.sh). This
// test proves that the gcsblob read path works: blob.OpenBucket("gs://...")
// with STORAGE_EMULATOR_HOST can list and stream objects.
//
// The full test matrix (no-logs, unauthorized, large-log, pipeline) runs
// under the S3 variant (logs_blob_test.go, build tag: blobs).
package e2e

import (
	"context"
	"net/http"
	"strings"
	"testing"

	resultsv1alpha2 "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

// TestGCSBlobLog_HappyPath creates a Result and Record that match the
// pre-seeded object path in fake-gcs-server, then reads via the v1alpha3
// plugin HTTP route and asserts the seeded content is returned.
func TestGCSBlobLog_HappyPath(t *testing.T) {
	ctx := context.Background()

	gc, _ := resultsClient(t, allNamespacesAdminAccessToken, nil)

	// Create Result + Record whose name segments match the seed path:
	//   logs/default/gcs-blob-test/gcs-blob-record/step-test
	res, err := gc.CreateResult(ctx, &resultsv1alpha2.CreateResultRequest{
		Parent: defaultNamespace,
		Result: &resultsv1alpha2.Result{
			Name: defaultNamespace + "/results/gcs-blob-test",
		},
	})
	if err != nil {
		t.Fatalf("CreateResult failed: %v", err)
	}
	t.Cleanup(func() {
		_, _ = gc.DeleteResult(ctx, &resultsv1alpha2.DeleteResultRequest{Name: res.GetName()})
	})

	rec, err := gc.CreateRecord(ctx, &resultsv1alpha2.CreateRecordRequest{
		Parent: res.GetName(),
		Record: &resultsv1alpha2.Record{
			Name: res.GetName() + "/records/gcs-blob-record",
			Data: &resultsv1alpha2.Any{
				Type:  "tekton.dev/v1.TaskRun",
				Value: []byte(`{}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateRecord failed: %v", err)
	}

	resultUID, recordUID := parseResultAndRecordIDs(res.GetName(), rec.GetName())
	t.Logf("Result: %s (%s), Record: %s (%s)", res.GetName(), resultUID, rec.GetName(), recordUID)

	statusCode, body := blobLogHTTPGet(t, defaultNamespace, resultUID, recordUID)
	if statusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d; body: %s", statusCode, body)
	}
	if !strings.Contains(body, "GCS_BLOB_MARKER_TEST") {
		t.Errorf("log body missing GCS_BLOB_MARKER_TEST; got: %.500s", body)
	}
	t.Logf("GCS blob log retrieval succeeded (%d bytes)", len(body))
}
