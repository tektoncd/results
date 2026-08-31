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
# Install Tekton Results with the bundled local Postgres, then apply the fixed
# performance tuning (pinned resources + postgresql.conf) for reproducible runs.
#
# This delegates the full app install (Pipelines, certs, tokens, ko deploy) to
# the e2e installer, then layers Postgres tuning on top imperatively so no ko
# rebuild is needed for the database.

set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="tekton-pipelines"

# Expose the API on localhost so the harness can reach it without port-forward
# gymnastics; the e2e installer honours this when generating the TLS cert.
export SSL_INCLUDE_LOCALHOST=${SSL_INCLUDE_LOCALHOST:-"true"}

echo "Installing Tekton Results (standard e2e deployment)..."
"${ROOT}/test/e2e/01-install.sh"

echo "Applying Postgres performance tuning..."
kubectl create configmap postgres-tuning \
    --namespace="${NAMESPACE}" \
    --from-file=postgresql.conf="${HERE}/postgresql.conf" \
    --dry-run=client -o yaml | kubectl apply -f -

kubectl patch statefulset postgres \
    --namespace="${NAMESPACE}" \
    --type=strategic \
    --patch-file "${HERE}/postgres-patch.yaml"

echo "Waiting for Postgres to roll out with tuning applied..."
kubectl rollout status statefulset/postgres --namespace="${NAMESPACE}" --timeout=180s
kubectl wait deployment tekton-results-api --namespace="${NAMESPACE}" --for=condition=available --timeout=120s

cat <<EOF

Tier-1 environment ready.

  API_SERVER_ADDR=https://localhost:8080
  SSL_CERT_PATH=${SSL_CERT_PATH:-/tmp/tekton-results/ssl}
  SA_TOKEN_PATH=${SA_TOKEN_PATH:-/tmp/tekton-results/tokens}

Run a benchmark, e.g.:

  go run ./test/performance/harness load --verify \\
    --cert "\${SSL_CERT_PATH}/tekton-results-cert.pem" \\
    --token "\${SA_TOKEN_PATH}/all-namespaces-admin-access"
EOF
