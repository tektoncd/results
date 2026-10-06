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
# Phase 3 — run the store / query / mixed benchmarks at tier 2 and write reports.
#
# Reports land under BENCH_OUTPUT_DIR (default test/performance/output/tier2) tagged
# with --tier tier2. The query driver replays the committed, KubeArchive-derived
# query mix by default (see harness/workload); override with --query-spec.
#
# The default BENCH_COUNT matches the small/ephemeral-cluster profile (see 02-load.sh
# and postgres-patch.yaml) so a run fits the bundled 1Gi Postgres PVC and the 3Gi DB
# node. A true 5-10 GB tier-2 baseline needs a larger BENCH_COUNT (~200k) plus a
# larger Postgres volume and DB node than the committed defaults provide — that
# capture is deferred to the ephemeral-cluster follow-up (see the plan/README).
#
# Tunables (env): BENCH_OUTPUT_DIR, BENCH_COUNT, BENCH_CONCURRENCY, BENCH_DURATION.

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(git rev-parse --show-toplevel)"
HARNESS="${ROOT}/test/performance/harness"
NAMESPACE="${BENCH_NAMESPACE:-tekton-pipelines}"
# shellcheck source=test/performance/environments/openshift/common.sh
source "${HERE}/common.sh"

bench::resolve_cluster "$@"
bench::start_timer
bench::phase "phase 3: benchmark (${CLUSTER})"

OUTPUT_DIR="${BENCH_OUTPUT_DIR:-${ROOT}/test/performance/output/tier2}"
mkdir -p "${OUTPUT_DIR}"

COUNT="${BENCH_COUNT:-10000}"
CONCURRENCY="${BENCH_CONCURRENCY:-64}"
DURATION="${BENCH_DURATION:-120}"
if [ "${CLUSTER}" = "kind" ]; then
    COUNT="${BENCH_COUNT:-2000}"
    CONCURRENCY="${BENCH_CONCURRENCY:-8}"
    DURATION="${BENCH_DURATION:-30}"
fi

SSL_CERT_PATH="${SSL_CERT_PATH:-/tmp/tekton-results/ssl}"
SA_TOKEN_PATH="${SA_TOKEN_PATH:-/tmp/tekton-results/tokens}"
CERT="${SSL_CERT_PATH}/tekton-results-cert.pem"
TOKEN="${SA_TOKEN_PATH}/all-namespaces-admin-access"
# The auth service accounts live in `default` (see test/e2e/kustomize/rbac.yaml),
# not the API namespace.
TOKEN_NAMESPACE="${BENCH_TOKEN_NAMESPACE:-default}"

common=(--cert "${CERT}" --token "${TOKEN}" --tier tier2 --count "${COUNT}" --concurrency "${CONCURRENCY}")

# Mint a token that outlives the run, then make the API reachable and pin the
# client to the deployment under test.
bench::refresh_token "${TOKEN_NAMESPACE}" all-namespaces-admin-access "${TOKEN}"
bench::api_forward "${NAMESPACE}"

bench::phase "phase 3: store"
go run "${HARNESS}" store "${common[@]}" --output "${OUTPUT_DIR}/store.json"

bench::phase "phase 3: query"
go run "${HARNESS}" query "${common[@]}" --transport both --duration "${DURATION}" --output "${OUTPUT_DIR}/query.json"

bench::phase "phase 3: mixed"
go run "${HARNESS}" mixed "${common[@]}" --duration "${DURATION}" --output "${OUTPUT_DIR}/mixed.json"

bench::phase "phase 3: benchmark complete (${CLUSTER})"
echo "Reports written to ${OUTPUT_DIR}"

# Validate that all reports are valid JSON (belt-and-suspenders; the harness would
# have errored on write if JSON serialization failed, but this catches any post-write
# corruption or incomplete files before an operator tries to compare them).
echo
echo "Validating reports..."
found=0
for report in "${OUTPUT_DIR}"/*.json; do
    [ -e "${report}" ] || continue
    # Skip the .sendlog.json sidecar (it's valid JSON but not a report).
    case "${report}" in *.sendlog.json) continue ;; esac
    found=1
    if bench::validate_json "${report}"; then
        echo "  ✓ $(basename "${report}")"
    else
        echo "  ✗ $(basename "${report}") is not valid JSON" >&2
        exit 1
    fi
done

if [ "${found}" -eq 0 ]; then
    echo "error: ${OUTPUT_DIR} contains no JSON reports; the benchmark may have failed" >&2
    exit 1
fi

cat <<EOF

========================================
Tier-2 benchmark complete (${CLUSTER})
========================================

Reports: ${OUTPUT_DIR}

Next steps:

  1. Compare against a baseline to detect regressions:

     go run ${HARNESS} compare \\
       <baseline.json> \\
       ${OUTPUT_DIR}/query.json

     (exits non-zero if p99_ms or throughput_per_sec regressed beyond the threshold)

  2. If this is your FIRST tier-2 run, save it as your baseline:

     COMMIT=\$(git rev-parse --short HEAD)
     mkdir -p ${ROOT}/test/performance/baselines/current
     cp ${OUTPUT_DIR}/query.json \\
        ${ROOT}/test/performance/baselines/current/tier2-seed-large-v1-\${COMMIT}.json

  3. Promote validated runs to the committed results tree:

     ${ROOT}/test/performance/results/<milestone>/tier2-seed-large-v1-<commit>.json

     See test/performance/results/README.md for the milestone-gate process.

EOF
