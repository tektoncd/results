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
# Phase 1 — install Tekton Results + Postgres for a tier-2 run.
#
# openshift: install the app, then apply the tier-2 Postgres tuning (dedicated-node
#            nodeSelector + larger postgresql.conf). Set BENCH_DB_URL to use an
#            external managed Postgres instead of the in-cluster one.
# kind:      delegate to the tier-1 local-DB installer; the tier-2 Postgres tuning
#            (dedicated node) does not apply on a single-node kind cluster.

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(git rev-parse --show-toplevel)"
NAMESPACE="${BENCH_NAMESPACE:-tekton-pipelines}"
# shellcheck source=test/performance/environments/openshift/common.sh
source "${HERE}/common.sh"

bench::resolve_cluster "$@"
bench::start_timer
bench::phase "phase 1: install (${CLUSTER})"

if [ "${CLUSTER}" = "kind" ]; then
    echo "kind mode: installing the tier-1 local-DB deployment."
    "${ROOT}/test/performance/environments/kind/01-install-localdb.sh"
    exit 0
fi

export SSL_INCLUDE_LOCALHOST="${SSL_INCLUDE_LOCALHOST:-true}"

if ! command -v oc >/dev/null 2>&1; then
    echo "error: oc is required to authenticate to the OpenShift image registry" >&2
    exit 1
fi

# Push locally built images to this cluster's integrated registry through a
# port-forward. The pod manifests use the registry's in-cluster service name.
export KO_DOCKER_REPO=localhost:5000/tekton-results
REGISTRY_TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/tekton-results-openshift.XXXXXX")"
REGISTRY_PORT_FORWARD_PID=""

bench::cleanup_registry_forward() {
    if [ -n "${REGISTRY_PORT_FORWARD_PID}" ]; then
        kill "${REGISTRY_PORT_FORWARD_PID}" 2>/dev/null || true
        wait "${REGISTRY_PORT_FORWARD_PID}" 2>/dev/null || true
    fi
    rm -rf -- "${REGISTRY_TMP_DIR}"
}
trap bench::cleanup_registry_forward EXIT

REGISTRY_FORWARD_LOG="${REGISTRY_TMP_DIR}/port-forward.log"
kubectl port-forward --namespace=openshift-image-registry service/image-registry 5000:5000 >"${REGISTRY_FORWARD_LOG}" 2>&1 &
REGISTRY_PORT_FORWARD_PID=$!
for _ in {1..30}; do
    if grep -Fq 'Forwarding from 127.0.0.1:5000' "${REGISTRY_FORWARD_LOG}"; then
        break
    fi
    if ! kill -0 "${REGISTRY_PORT_FORWARD_PID}" 2>/dev/null; then
        cat "${REGISTRY_FORWARD_LOG}" >&2
        exit 1
    fi
    sleep 1
done
if ! grep -Fq 'Forwarding from 127.0.0.1:5000' "${REGISTRY_FORWARD_LOG}"; then
    cat "${REGISTRY_FORWARD_LOG}" >&2
    echo "error: timed out waiting for the OpenShift registry port-forward" >&2
    exit 1
fi

DOCKER_CONFIG="${REGISTRY_TMP_DIR}/docker"
mkdir -p "${DOCKER_CONFIG}"
oc registry login --registry=localhost:5000 --insecure --skip-check --to="${DOCKER_CONFIG}/config.json"
export DOCKER_CONFIG

# On OpenShift the Tekton and Results pods are rejected by the default
# restricted-v2 SCC for two independent reasons: they request UID 65532 (outside
# the namespace's allocated UID range) and they set the deprecated
# container.seccomp.security.alpha.kubernetes.io annotation. The privileged SCC
# admits both; anyuid does not (it rejects the seccomp annotation). Grant it to
# every service account in the namespace up front, so pods are admitted the first
# time they are scheduled.
echo "OpenShift detected: granting privileged SCC to service accounts in ${NAMESPACE}..."
oc create namespace "${NAMESPACE}" 2>/dev/null || true
oc adm policy add-scc-to-group privileged "system:serviceaccounts:${NAMESPACE}"
oc create namespace tekton-results 2>/dev/null || true
oc adm policy add-role-to-group system:image-puller "system:serviceaccounts:${NAMESPACE}" --namespace=tekton-results

# Clear any database reservation left by a previous run before the app is deployed,
# so the fresh Postgres pod is not repelled by a stale taint and lands on a node we
# can then reserve deterministically.
if [ -z "${BENCH_DB_URL:-}" ]; then
    bench::clear_db_node_reservation
fi

# The app-install steps below (Pipelines, DB secret, TLS cert, tokens) mirror
# test/e2e/01-install.sh, but this path deliberately diverges: it publishes images
# to the in-cluster OpenShift registry and rewrites the manifest to the internal
# service name, so it cannot simply call the e2e installer's `ko apply` flow.
echo "Installing Tekton Pipelines..."
TEKTON_PIPELINE_CONFIG="${TEKTON_PIPELINE_CONFIG:-https://infra.tekton.dev/tekton-releases/pipeline/latest/release.yaml}"
bench::kubectl apply --filename "${TEKTON_PIPELINE_CONFIG}"
bench::kubectl wait --for=condition=ready pod -l app=tekton-pipelines-controller -n tekton-pipelines --timeout=120s
bench::kubectl wait --for=condition=ready pod -l app=tekton-pipelines-webhook -n tekton-pipelines --timeout=120s

echo "Generating DB secret..."
bench::kubectl create secret generic tekton-results-postgres \
    --namespace="${NAMESPACE}" \
    --from-literal=POSTGRES_USER=postgres \
    --from-literal=POSTGRES_PASSWORD="$(openssl rand -base64 20)" || true

echo "Generating TLS key pair..."
SSL_CERT_PATH="${SSL_CERT_PATH:-/tmp/tekton-results/ssl}"
SA_TOKEN_PATH="${SA_TOKEN_PATH:-/tmp/tekton-results/tokens}"
mkdir -p "${SSL_CERT_PATH}"
export EXTRA_SANS=""
export OUTPUT_DIR="${SSL_CERT_PATH}"
export CERT_FILE_NAME="tekton-results-cert.pem"
export KEY_FILE_NAME="tekton-results-key.pem"
"${ROOT}/config/components/horizontal-scaling/generate-tls-cert.sh"

echo "Building and publishing Tekton Results images to the OpenShift registry..."
RENDERED_MANIFEST="${REGISTRY_TMP_DIR}/tekton-results.yaml"
bench::kubectl kustomize "${ROOT}/test/e2e/kustomize" |
    ko resolve --insecure-registry --platform="linux/$(go env GOARCH)" -f - >"${RENDERED_MANIFEST}"
kill "${REGISTRY_PORT_FORWARD_PID}" 2>/dev/null || true
wait "${REGISTRY_PORT_FORWARD_PID}" 2>/dev/null || true
REGISTRY_PORT_FORWARD_PID=""
INTERNAL_MANIFEST="${REGISTRY_TMP_DIR}/tekton-results-internal.yaml"
sed 's#localhost:5000/tekton-results#image-registry.openshift-image-registry.svc:5000/tekton-results#g' \
    "${RENDERED_MANIFEST}" >"${INTERNAL_MANIFEST}"
mv "${INTERNAL_MANIFEST}" "${RENDERED_MANIFEST}"
if grep -Fq 'localhost:5000/' "${RENDERED_MANIFEST}"; then
    echo "error: rendered manifest still contains a workstation-only image reference" >&2
    exit 1
fi
bench::kubectl apply -f "${RENDERED_MANIFEST}"

# The API server runs an RBAC check (TokenReview + SubjectAccessReview against the
# kube-apiserver) on every gRPC call, using a client-go client capped by K8S_QPS/
# K8S_BURST (defaults 5/10, see config/base/env/config). At the concurrency this
# benchmark drives, that default throttles the API server's own outbound auth calls
# to ~5/sec — the true store-throughput ceiling, visible as "client-side throttling,
# not priority and fairness" delays in the API log. Raise it so auth is not the
# bottleneck; the apiserver's API Priority & Fairness still protects it server-side.
# The explicit env entries override the values baked into the config ConfigMap.
API_K8S_QPS="${BENCH_API_K8S_QPS:-100}"
API_K8S_BURST="${BENCH_API_K8S_BURST:-200}"
echo "Raising API server kube-client rate limits (K8S_QPS=${API_K8S_QPS}, K8S_BURST=${API_K8S_BURST})..."
bench::kubectl set env deployment/tekton-results-api --namespace="${NAMESPACE}" \
    "K8S_QPS=${API_K8S_QPS}" "K8S_BURST=${API_K8S_BURST}"

echo "Fetching access tokens..."
mkdir -p "${SA_TOKEN_PATH}"
service_accounts=(all-namespaces-read-access single-namespace-read-access all-namespaces-admin-access all-namespaces-impersonate-access)
for service_account in "${service_accounts[@]}"; do
    bench::kubectl create token "${service_account}" >"${SA_TOKEN_PATH}/${service_account}"
    echo "Created ${SA_TOKEN_PATH}/${service_account}"
done

echo "Waiting for Tekton Results pods..."
bench::kubectl wait pod tekton-results-postgres-0 --namespace="${NAMESPACE}" --for=condition=Ready --timeout=120s
bench::kubectl wait deployment tekton-results-api --namespace="${NAMESPACE}" --for=condition=available --timeout=300s
bench::kubectl wait deployment tekton-results-watcher --namespace="${NAMESPACE}" --for=condition=available --timeout=300s
bench::kubectl wait deployment tekton-results-retention-policy-agent --namespace="${NAMESPACE}" --for=condition=available --timeout=300s

if [ -n "${BENCH_DB_URL:-}" ]; then
    echo "External database configured via BENCH_DB_URL; skipping in-cluster Postgres tuning."
    echo "Ensure the API server Secret points at the external instance."
else
    echo "Applying tier-2 Postgres tuning (dedicated node + tuned postgresql.conf)..."
    DATABASE_NODE="$(bench::kubectl get pod tekton-results-postgres-0 \
        --namespace="${NAMESPACE}" -o jsonpath='{.spec.nodeName}')"
    if [ -z "${DATABASE_NODE}" ]; then
        echo "error: could not determine the node hosting tekton-results-postgres-0" >&2
        exit 1
    fi
    # Fail fast if the node is too small for the patched Postgres request, instead
    # of wedging on an un-schedulable pod. Keep the default in sync with the
    # requests.memory in postgres-patch.yaml.
    bench::require_node_memory "${DATABASE_NODE}" "${BENCH_DB_MIN_MEMORY:-2Gi}"

    echo "Reserving ${DATABASE_NODE} for Postgres..."
    # Order matters: label first so the nodeSelector can match, then patch (which
    # adds the nodeSelector, tolerations, and resources and reschedules Postgres onto
    # this node), and only taint once Postgres tolerates it — otherwise a NoExecute
    # taint would evict the very pod we are placing.
    bench::kubectl label node "${DATABASE_NODE}" tekton-results.dev/role=database --overwrite

    bench::kubectl create configmap postgres-tuning \
        --namespace="${NAMESPACE}" \
        --from-file=postgresql.conf="${HERE}/postgresql.conf" \
        --dry-run=client -o yaml | bench::kubectl apply -f -

    bench::kubectl patch statefulset tekton-results-postgres \
        --namespace="${NAMESPACE}" \
        --type=strategic \
        --patch-file "${HERE}/postgres-patch.yaml"

    # Taint before waiting for the rollout: the patch already gave the new Postgres
    # pod the matching tolerations, so NoExecute (the default) evicts competing
    # workloads and frees the node the pod needs. Tainting after the wait could
    # deadlock — the larger pod might stay Pending behind pods that never get evicted.
    # Override the effect with BENCH_DB_TAINT_EFFECT (e.g. NoSchedule to keep others).
    DB_TAINT_EFFECT="${BENCH_DB_TAINT_EFFECT:-NoExecute}"
    bench::kubectl taint node "${DATABASE_NODE}" "tekton-results.dev/role=database:${DB_TAINT_EFFECT}" --overwrite

    echo "Waiting for Postgres to roll out onto the reserved node..."
    bench::kubectl rollout status statefulset/tekton-results-postgres --namespace="${NAMESPACE}" --timeout=300s
fi

bench::kubectl wait deployment tekton-results-api --namespace="${NAMESPACE}" --for=condition=available --timeout=180s

cat <<EOF

Phase 1 complete (cluster=${CLUSTER}).

Set the connection env before loading:
  export API_SERVER_ADDR=<api route or forwarded address>
  export SSL_CERT_PATH=/tmp/tekton-results/ssl
  export SA_TOKEN_PATH=/tmp/tekton-results/tokens
EOF
