#!/usr/bin/env bash
# Copyright 2026 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# Phase 2 — load the tier-2 seed dataset through the API.
#
# The committed defaults below are the small/ephemeral-cluster profile that fits the
# bundled 1Gi Postgres PVC and 3Gi DB node. A full 5-10 GB tier-2 baseline needs a
# larger BENCH_COUNT plus a larger Postgres volume and DB node, and is deferred to
# the ephemeral-cluster follow-up (see the plan/README).
#
# By default this loads via the real API path at high concurrency (the honest,
# code-exercising path). For a fast reset between back-to-back runs you may restore
# a previously captured pg_dump snapshot instead by setting BENCH_SEED_DUMP — this
# bypasses the API entirely, so it is valid ONLY to seed the read (query) dataset,
# never as input to a store benchmark.
#
# Throughput ceiling: the API server runs an RBAC check (TokenReview +
# SubjectAccessReview against the kube-apiserver) on EVERY gRPC call, and each
# PipelineRun is ~7 calls. That auth path — not Postgres or --concurrency — is the
# store bottleneck, capped by the API server's client-go rate limits (K8S_QPS/
# K8S_BURST). 01-install.sh raises them (default 100/200) so auth keeps up; if you
# see "client-side throttling" delays in the API log, raise BENCH_API_K8S_QPS and
# re-install. Beyond that, load time scales linearly with BENCH_COUNT, so size it to
# the ~90-min budget.
#
# Tunables (env): BENCH_COUNT, BENCH_CONCURRENCY, BENCH_NAMESPACES, BENCH_SEED_DUMP.

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(git rev-parse --show-toplevel)"
HARNESS="${ROOT}/test/performance/harness"
NAMESPACE="${BENCH_NAMESPACE:-tekton-pipelines}"
# shellcheck source=test/performance/environments/openshift/common.sh
source "${HERE}/common.sh"

bench::resolve_cluster "$@"
bench::start_timer
bench::phase "phase 2: load (${CLUSTER})"

# Tier-2 defaults scale far above tier-1; override for a quick kind dry run.
COUNT="${BENCH_COUNT:-10000}"
CONCURRENCY="${BENCH_CONCURRENCY:-64}"
NAMESPACES="${BENCH_NAMESPACES:-20}"
if [ "${CLUSTER}" = "kind" ]; then
    COUNT="${BENCH_COUNT:-2000}"
    CONCURRENCY="${BENCH_CONCURRENCY:-8}"
    NAMESPACES="${BENCH_NAMESPACES:-50}"
fi

SSL_CERT_PATH="${SSL_CERT_PATH:-/tmp/tekton-results/ssl}"
SA_TOKEN_PATH="${SA_TOKEN_PATH:-/tmp/tekton-results/tokens}"
CERT="${SSL_CERT_PATH}/tekton-results-cert.pem"
TOKEN="${SA_TOKEN_PATH}/all-namespaces-admin-access"
# The auth service accounts live in `default` (see test/e2e/kustomize/rbac.yaml),
# not the API namespace.
TOKEN_NAMESPACE="${BENCH_TOKEN_NAMESPACE:-default}"

if [ -n "${BENCH_SEED_DUMP:-}" ]; then
    echo "Restoring seed dataset from pg_dump snapshot ${BENCH_SEED_DUMP} (query-only seed; bypasses the API)..."
    # The bundled Postgres is a StatefulSet, so the pod name is deterministic.
    pod="tekton-results-postgres-0"
    if ! bench::kubectl get pod "${pod}" --namespace="${NAMESPACE}" >/dev/null 2>&1; then
        echo "error: ${pod} not found in ${NAMESPACE}; run 01-install.sh first" >&2
        exit 1
    fi
    bench::kubectl exec -i --namespace="${NAMESPACE}" "${pod}" -- \
        psql -U postgres -d tekton-results <"${BENCH_SEED_DUMP}"
    echo "Snapshot restored. Reminder: this seed is valid for query runs only."
    exit 0
fi

# Mint a token that outlives the run, then make the API reachable and pin the
# client to the deployment under test.
bench::refresh_token "${TOKEN_NAMESPACE}" all-namespaces-admin-access "${TOKEN}"
bench::api_forward "${NAMESPACE}"

echo "Loading ${COUNT} PipelineRuns across ${NAMESPACES} namespaces at concurrency ${CONCURRENCY}..."
go run "${HARNESS}" load --verify \
    --cert "${CERT}" --token "${TOKEN}" \
    --count "${COUNT}" \
    --concurrency "${CONCURRENCY}" \
    --namespaces "${NAMESPACES}" \
    --dataset seed-large --dataset-version seed-large-v1

bench::phase "phase 2: load complete (${CLUSTER})"
