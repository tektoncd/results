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
# Functional smoke run (NOT a performance gate): load the seed dataset and run all
# three modes at a small scale, asserting each exits 0 and emits valid JSON. Meant
# for CI against a freshly installed tier-1 kind cluster (01-install-localdb.sh).

set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
HARNESS="${ROOT}/test/performance/harness"
OUT="$(mktemp -d)"
trap 'rm -rf "${OUT}"' EXIT

SSL_CERT_PATH=${SSL_CERT_PATH:-"/tmp/tekton-results/ssl"}
SA_TOKEN_PATH=${SA_TOKEN_PATH:-"/tmp/tekton-results/tokens"}
CERT="${SSL_CERT_PATH}/tekton-results-cert.pem"
TOKEN="${SA_TOKEN_PATH}/all-namespaces-admin-access"

COUNT=${SMOKE_COUNT:-1000}
DURATION=${SMOKE_DURATION:-15}

common=(--cert "${CERT}" --token "${TOKEN}" --count "${COUNT}")

assert_json() {
    local file="$1"
    if ! python3 -c "import json,sys; json.load(open(sys.argv[1]))" "${file}"; then
        echo "FAIL: ${file} is not valid JSON" >&2
        exit 1
    fi
    echo "ok: ${file}"
}

echo "== load --verify =="
go run "${HARNESS}" load --verify "${common[@]}" --output "${OUT}/load.json"
assert_json "${OUT}/load.json"

echo "== store =="
go run "${HARNESS}" store "${common[@]}" --output "${OUT}/store.json"
assert_json "${OUT}/store.json"

echo "== query =="
go run "${HARNESS}" query "${common[@]}" --transport both --duration "${DURATION}" --output "${OUT}/query.json"
assert_json "${OUT}/query.json"

echo "== mixed =="
go run "${HARNESS}" mixed "${common[@]}" --duration "${DURATION}" --output "${OUT}/mixed.json"
assert_json "${OUT}/mixed.json"

echo "smoke run passed"
