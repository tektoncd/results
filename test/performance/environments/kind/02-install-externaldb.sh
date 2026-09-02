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
# Install Tekton Results against an EXTERNAL Postgres, so benchmarks can target a
# managed/production-grade database instead of the in-cluster bitnami instance.
#
# The single input is BENCH_DB_URL:
#
#   export BENCH_DB_URL="postgres://user:pass@host:5432/results?sslmode=require"
#   ./02-install-externaldb.sh
#
# The API server already reads DB_* from its config file and DB_USER/DB_PASSWORD
# from a secret, so this script only rewires configuration — no code or image
# changes. The harness itself never touches the database, so it is unchanged.

set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
NAMESPACE="tekton-pipelines"

export KO_DOCKER_REPO=${KO_DOCKER_REPO:-"kind.local"}
export SA_TOKEN_PATH=${SA_TOKEN_PATH:-"/tmp/tekton-results/tokens"}
export SSL_CERT_PATH=${SSL_CERT_PATH:-"/tmp/tekton-results/ssl"}
export SSL_INCLUDE_LOCALHOST=${SSL_INCLUDE_LOCALHOST:-"true"}

if [ -z "${BENCH_DB_URL:-}" ]; then
    echo "BENCH_DB_URL is required, e.g. postgres://user:pass@host:5432/db?sslmode=require" >&2
    exit 1
fi

# Parse BENCH_DB_URL into the discrete DB_* settings the API expects.
# Format: <scheme>://<user>:<pass>@<host>:<port>/<db>[?sslmode=<mode>]
proto_stripped="${BENCH_DB_URL#*://}"
creds="${proto_stripped%%@*}"
hostpart="${proto_stripped#*@}"
DB_USER="${creds%%:*}"
DB_PASSWORD="${creds#*:}"
hostport="${hostpart%%/*}"
DB_HOST="${hostport%%:*}"
DB_PORT="${hostport##*:}"
[ "${DB_PORT}" = "${DB_HOST}" ] && DB_PORT="5432" # no explicit port
pathpart="${hostpart#*/}"
DB_NAME="${pathpart%%\?*}"
if [[ "${BENCH_DB_URL}" == *"sslmode="* ]]; then
    DB_SSLMODE="${BENCH_DB_URL##*sslmode=}"
    DB_SSLMODE="${DB_SSLMODE%%&*}"
else
    DB_SSLMODE="require"
fi

if [ -z "${DB_HOST}" ] || [ -z "${DB_NAME}" ] || [ -z "${DB_USER}" ]; then
    echo "could not parse BENCH_DB_URL (need host, db name and user)" >&2
    exit 1
fi

echo "External DB target: ${DB_USER}@${DB_HOST}:${DB_PORT}/${DB_NAME} (sslmode=${DB_SSLMODE})"

echo "Installing Tekton Pipelines..."
TEKTON_PIPELINE_CONFIG=${TEKTON_PIPELINE_CONFIG:-"https://infra.tekton.dev/tekton-releases/pipeline/latest/release.yaml"}
kubectl apply --filename "${TEKTON_PIPELINE_CONFIG}"

echo "Creating DB credentials secret..."
kubectl create secret generic tekton-results-postgres \
    --namespace="${NAMESPACE}" \
    --from-literal=POSTGRES_USER="${DB_USER}" \
    --from-literal=POSTGRES_PASSWORD="${DB_PASSWORD}" \
    --dry-run=client -o yaml | kubectl apply -f -

echo "Generating TLS key pair..."
mkdir -p "${SSL_CERT_PATH}"
altNames="DNS:tekton-results-api-service.${NAMESPACE}.svc.cluster.local,DNS:localhost"
openssl req -x509 \
    -newkey rsa:4096 \
    -keyout "${SSL_CERT_PATH}/tekton-results-key.pem" \
    -out "${SSL_CERT_PATH}/tekton-results-cert.pem" \
    -days 365 -nodes \
    -subj "/CN=tekton-results-api-service.${NAMESPACE}.svc.cluster.local" \
    -addext "subjectAltName = ${altNames}"
kubectl create secret tls tekton-results-tls \
    --namespace="${NAMESPACE}" \
    --cert="${SSL_CERT_PATH}/tekton-results-cert.pem" \
    --key="${SSL_CERT_PATH}/tekton-results-key.pem" \
    --dry-run=client -o yaml | kubectl apply -f -

echo "Deploying Tekton Results (base-only, no in-cluster DB)..."
extra_ko_params="linux/$(go env GOARCH)"
kubectl kustomize "${ROOT}/config/overlays/base-only" | ko apply --platform="${extra_ko_params}" -f -

echo "Applying benchmark RBAC (service accounts + access) ..."
kubectl apply -f "${ROOT}/test/e2e/kustomize/rbac.yaml"

echo "Exposing the API on NodePort 30080..."
kubectl patch service tekton-results-api-service \
    --namespace="${NAMESPACE}" \
    --type=json \
    --patch "$(cat "${ROOT}/test/e2e/kustomize/api-service.yaml")"

echo "Rewriting api-config with external DB settings..."
config_tmp="$(mktemp)"
trap 'rm -f "${config_tmp}"' EXIT
sed -e "s|^DB_HOST=.*|DB_HOST=${DB_HOST}|" \
    -e "s|^DB_PORT=.*|DB_PORT=${DB_PORT}|" \
    -e "s|^DB_NAME=.*|DB_NAME=${DB_NAME}|" \
    -e "s|^DB_SSLMODE=.*|DB_SSLMODE=${DB_SSLMODE}|" \
    "${ROOT}/config/base/env/config" > "${config_tmp}"
kubectl create configmap api-config \
    --namespace="${NAMESPACE}" \
    --from-file=config="${config_tmp}" \
    --dry-run=client -o yaml | kubectl apply -f -

echo "Restarting the API to pick up external DB settings..."
kubectl rollout restart deployment/tekton-results-api --namespace="${NAMESPACE}"

echo "Fetching access tokens..."
mkdir -p "${SA_TOKEN_PATH}"
for sa in all-namespaces-read-access single-namespace-read-access all-namespaces-admin-access all-namespaces-impersonate-access; do
    kubectl create token "${sa}" > "${SA_TOKEN_PATH}/${sa}"
done

echo "Waiting for the API to be ready..."
kubectl rollout status deployment/tekton-results-api --namespace="${NAMESPACE}" --timeout=180s

cat <<EOF

External-DB environment ready (DB_BACKEND=external).

  API_SERVER_ADDR=https://localhost:8080
  BENCH_DB_BACKEND=external
  SSL_CERT_PATH=${SSL_CERT_PATH}
  SA_TOKEN_PATH=${SA_TOKEN_PATH}
EOF
