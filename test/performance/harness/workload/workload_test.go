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

package workload

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseRequest(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		want   request
		wantOK bool
	}{
		{
			name:   "namespaced list with label selector",
			line:   `status=200 GET /apis/tekton.dev/v1/namespaces/tenant-a/pipelineruns?limit=30&labelSelector=appstudio.openshift.io/application=alpha`,
			want:   request{op: OpLabelSelectorList, kind: "pipelineruns", namespace: "tenant-a", limit: 30, labelKeys: []string{"appstudio.openshift.io/application"}, isCollection: true},
			wantOK: true,
		},
		{
			name:   "structured slog line, plain list",
			line:   `time=2026-09-24T10:00:02.100Z level=INFO source=.../logging.go:45 msg="Served request" method=GET status=200 latency=994ms client=10.29.64.69 path="/apis/tekton.dev/v1/namespaces/tenant-a/pipelineruns?creationTimestampAfter=2026-09-22T22%3A23%3A51Z&limit=500" errors="" trace-id=abc span-id=def`,
			want:   request{op: OpListByNamespaceKind, kind: "pipelineruns", namespace: "tenant-a", limit: 500, isCollection: true},
			wantOK: true,
		},
		{
			name:   "structured slog line, url-encoded label selector",
			line:   `time=2026-09-24T10:00:10.180Z level=INFO msg="Served request" method=GET status=200 path="/apis/tekton.dev/v1/namespaces/tenant-a/pipelineruns?labelSelector=appstudio.openshift.io%2Fapplication%3Dalpha&limit=30" trace-id=abc`,
			want:   request{op: OpLabelSelectorList, kind: "pipelineruns", namespace: "tenant-a", limit: 30, labelKeys: []string{"appstudio.openshift.io/application"}, isCollection: true},
			wantOK: true,
		},
		{
			name:   "structured slog write verb ignored",
			line:   `time=2026-09-24T10:00:30.380Z level=INFO msg="Served request" method=POST status=201 path="/apis/tekton.dev/v1/namespaces/ns/pipelineruns" trace-id=abc`,
			wantOK: false,
		},
		{
			name:   "klog throttling line ignored",
			line:   `I0924 10:00:08.500000       1 request.go:752] "Waited before sending request" reason="client-side throttling" verb="POST" URL="https://172.30.0.1:443/apis/authorization.k8s.io/v1/subjectaccessreviews"`,
			wantOK: false,
		},
		{
			name:   "namespaced plain list",
			line:   `GET /apis/tekton.dev/v1/namespaces/tenant-b/taskruns?limit=100&continue=`,
			want:   request{op: OpListByNamespaceKind, kind: "taskruns", namespace: "tenant-b", limit: 100, isCollection: true},
			wantOK: true,
		},
		{
			name:   "get by name",
			line:   `GET /apis/tekton.dev/v1/namespaces/tenant-a/pipelineruns/my-run-123`,
			want:   request{op: OpGetByName, kind: "pipelineruns", namespace: "tenant-a"},
			wantOK: true,
		},
		{
			// KubeArchive serves by-uid through a dedicated /uid/<uid> route
			// (cmd/api/main.go), not a metadata.uid field selector.
			name:   "uid lookup via /uid/ route",
			line:   `GET /apis/tekton.dev/v1/namespaces/tenant-a/pipelineruns/uid/abc-123`,
			want:   request{op: OpOwnerUIDLookup, kind: "pipelineruns", namespace: "tenant-a"},
			wantOK: true,
		},
		{
			// fieldSelector is not a KubeArchive query param; the path is just a
			// plain namespaced list and any fieldSelector is ignored.
			name:   "field selector is ignored, treated as plain list",
			line:   `GET /apis/tekton.dev/v1/namespaces/tenant-a/pipelineruns?fieldSelector=metadata.uid=abc-123`,
			want:   request{op: OpListByNamespaceKind, kind: "pipelineruns", namespace: "tenant-a", isCollection: true},
			wantOK: true,
		},
		{
			// KubeArchive treats results.tekton.dev/childOfUid as an ordinary
			// label, so an owner-child fan-out is a label-selector list.
			name:   "childOfUid label is a label-selector list",
			line:   `GET /apis/tekton.dev/v1/namespaces/tenant-a/pipelineruns?labelSelector=results.tekton.dev/childOfUid=xyz`,
			want:   request{op: OpLabelSelectorList, kind: "pipelineruns", namespace: "tenant-a", labelKeys: []string{"results.tekton.dev/childOfUid"}, isCollection: true},
			wantOK: true,
		},
		{
			name:   "log subresource ignored",
			line:   `GET /apis/tekton.dev/v1/namespaces/tenant-a/pipelineruns/my-run-123/log`,
			wantOK: false,
		},
		{
			name:   "cluster scoped list",
			line:   `GET /apis/tekton.dev/v1/pipelineruns?limit=500`,
			want:   request{op: OpListByNamespaceKind, kind: "pipelineruns", limit: 500, isCollection: true},
			wantOK: true,
		},
		{
			name:   "set-based selector",
			line:   `GET /apis/tekton.dev/v1/namespaces/ns/pipelineruns?labelSelector=appstudio.openshift.io/application in (a,b)`,
			want:   request{op: OpLabelSelectorList, kind: "pipelineruns", namespace: "ns", labelKeys: []string{"appstudio.openshift.io/application"}, isCollection: true},
			wantOK: true,
		},
		{name: "health check ignored", line: `GET /livez`, wantOK: false},
		{name: "metrics ignored", line: `GET /metrics`, wantOK: false},
		{name: "write verb ignored", line: `POST /apis/tekton.dev/v1/namespaces/ns/pipelineruns`, wantOK: false},
		{name: "non-request line ignored", line: `starting server on :8080`, wantOK: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseRequest(tc.line)
			if ok != tc.wantOK {
				t.Fatalf("parseRequest(%q) ok = %v, want %v", tc.line, ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if diff := cmp.Diff(tc.want, got, cmp.AllowUnexported(request{})); diff != "" {
				t.Errorf("parseRequest(%q) mismatch (-want +got):\n%s", tc.line, diff)
			}
		})
	}
}

func TestDeriveFromSample(t *testing.T) {
	f, err := os.Open("testdata/sample-access.log")
	if err != nil {
		t.Fatalf("opening sample: %v", err)
	}
	defer func() { _ = f.Close() }()

	spec, err := Derive(f)
	if err != nil {
		t.Fatalf("Derive() error: %v", err)
	}

	if got, want := spec.SchemaVersion, SchemaVersion; got != want {
		t.Errorf("SchemaVersion = %q, want %q", got, want)
	}
	// 28 GET API requests in the sample; health/metrics paths, the POST, and the
	// non-access-log noise (klog throttling, http2 preface, trace-export) excluded.
	if got, want := spec.SampleSize, 28; got != want {
		t.Errorf("SampleSize = %d, want %d", got, want)
	}

	// Operations are ordered by descending frequency; plain namespaced pipelinerun
	// lists (the polling loop) dominate.
	if len(spec.Operations) == 0 || spec.Operations[0].Type != OpListByNamespaceKind {
		t.Errorf("top operation = %+v, want %s", spec.Operations[0], OpListByNamespaceKind)
	}
	total := 0
	for _, op := range spec.Operations {
		total += op.Count
	}
	if total != spec.SampleSize {
		t.Errorf("operation counts sum to %d, want SampleSize %d", total, spec.SampleSize)
	}

	// Page sizes present and ordered by frequency (limit=500 most common).
	if len(spec.PageSizes) == 0 || spec.PageSizes[0].Limit != 500 {
		t.Errorf("top page size = %+v, want limit 500", spec.PageSizes[0])
	}

	if spec.NamespaceFanout.DistinctCount != 5 {
		t.Errorf("DistinctCount = %d, want 5", spec.NamespaceFanout.DistinctCount)
	}
	if spec.LabelSelector.Fraction <= 0 || spec.LabelSelector.Fraction > 1 {
		t.Errorf("LabelSelector.Fraction = %v, want (0,1]", spec.LabelSelector.Fraction)
	}

	if err := spec.Validate(); err != nil {
		t.Errorf("Validate() error: %v", err)
	}
}

// TestDeriveAnonymizes is the security-critical guarantee: no raw identifier from
// the source log may leak into the derived spec.
func TestDeriveAnonymizes(t *testing.T) {
	f, err := os.Open("testdata/sample-access.log")
	if err != nil {
		t.Fatalf("opening sample: %v", err)
	}
	defer func() { _ = f.Close() }()

	spec, err := Derive(f)
	if err != nil {
		t.Fatalf("Derive() error: %v", err)
	}
	var buf bytes.Buffer
	if err := Write(&buf, spec); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	rendered := buf.String()

	forbidden := []string{
		// namespaces
		"acme-tenant", "globex-tenant", "initech-tenant", "umbrella-tenant", "wayne-tenant",
		// label values
		"acme-app", "globex-app", "billing-frontend", "pull_request",
		// object names
		"on-push-run", "on-pull-run", "acme-app-run-build",
		// uids
		"cafef00d",
		// client IPs and trace/span IDs
		"10.29.64.69", "d9c0ba1c",
	}
	for _, s := range forbidden {
		if strings.Contains(rendered, s) {
			t.Errorf("derived spec leaks raw identifier %q:\n%s", s, rendered)
		}
	}
}

func TestSpecRoundTrip(t *testing.T) {
	f, err := os.Open("testdata/sample-access.log")
	if err != nil {
		t.Fatalf("opening sample: %v", err)
	}
	defer func() { _ = f.Close() }()
	want, err := Derive(f)
	if err != nil {
		t.Fatalf("Derive() error: %v", err)
	}

	var buf bytes.Buffer
	if err := Write(&buf, want); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	got, err := Load(buf.Bytes())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("round-trip mismatch (-want +got):\n%s", diff)
	}
}

func TestDefaultSpec(t *testing.T) {
	spec, err := Default()
	if err != nil {
		t.Fatalf("Default() error: %v", err)
	}
	if err := spec.Validate(); err != nil {
		t.Errorf("embedded default spec invalid: %v", err)
	}
}
