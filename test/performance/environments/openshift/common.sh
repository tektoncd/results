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
# Shared helpers for the tier-2 benchmark phase scripts: --cluster resolution and
# 90-minute phase-budget timing. Source this file; do not execute it.
#
# The tier-2 pipeline targets an ephemeral multi-node OpenShift cluster, but every
# phase also accepts `--cluster kind` so the whole flow can be dry-run locally
# before booking OpenShift time. The chosen value is exported as BENCH_CLUSTER so
# it threads through the phases when they are run in the same shell; each phase
# also accepts its own --cluster flag.

# BENCH_BUDGET_SECONDS is the tier-2 session budget (default 90 minutes). Phase
# banners warn once elapsed time crosses it.
BENCH_BUDGET_SECONDS="${BENCH_BUDGET_SECONDS:-5400}"

# bench::resolve_cluster parses --cluster from the given args (falling back to the
# BENCH_CLUSTER env var, default "openshift"), validates it, and exports the result
# as both BENCH_CLUSTER and the CLUSTER shell variable.
bench::resolve_cluster() {
    local cluster="${BENCH_CLUSTER:-openshift}"
    while [ $# -gt 0 ]; do
        case "$1" in
        --cluster)
            cluster="${2:-}"
            shift 2 || shift
            ;;
        --cluster=*)
            cluster="${1#*=}"
            shift
            ;;
        *)
            shift
            ;;
        esac
    done
    case "${cluster}" in
    kind | openshift) ;;
    *)
        echo "error: --cluster must be 'kind' or 'openshift' (got '${cluster}')" >&2
        return 1
        ;;
    esac
    export BENCH_CLUSTER="${cluster}"
    # CLUSTER is consumed by the phase scripts that source this file.
    # shellcheck disable=SC2034
    CLUSTER="${cluster}"
}

# bench::start_timer records the phase-timer origin for this script.
bench::start_timer() {
    BENCH_T0="$(date +%s)"
}

# bench::phase prints a banner with elapsed time and warns when the budget is
# exceeded, so an operator can keep the whole session inside the 90-minute window.
bench::phase() {
    local name="$1"
    : "${BENCH_T0:=$(date +%s)}"
    local now elapsed
    now="$(date +%s)"
    elapsed=$((now - BENCH_T0))
    printf '== %s == (elapsed %dm%02ds / budget %dm)\n' \
        "${name}" $((elapsed / 60)) $((elapsed % 60)) $((BENCH_BUDGET_SECONDS / 60))
    if [ "${elapsed}" -gt "${BENCH_BUDGET_SECONDS}" ]; then
        echo "warning: tier-2 session budget of $((BENCH_BUDGET_SECONDS / 60)) minutes exceeded (${elapsed}s elapsed)" >&2
    fi
}

# bench::kubectl runs kubectl (OpenShift's oc is compatible for the verbs used
# here); callers rely on the active KUBECONFIG context.
bench::kubectl() {
    kubectl "$@"
}

# bench::refresh_token mints a fresh Results SA token that outlives the session
# budget and writes it to the given path, so long tier-2 runs never fail mid-flight
# on an expired token. `kubectl create token` defaults to a 1h TTL — shorter than the
# 90-minute budget — so we request BENCH_BUDGET_SECONDS plus a margin. The apiserver
# silently clamps the duration to its --service-account-max-token-expiration if the
# request is larger, which is still safe as long as it exceeds the run length.
#
# Usage: bench::refresh_token "${TOKEN_NAMESPACE}" all-namespaces-admin-access "${TOKEN}"
bench::refresh_token() {
    local namespace="${1:-default}"
    local sa="${2:?service account name required}"
    local out="${3:?token output path required}"
    local margin=1800
    local duration=$((BENCH_BUDGET_SECONDS + margin))
    echo "Minting ${namespace}/${sa} token (duration ${duration}s) into ${out}..."
    mkdir -p "$(dirname "${out}")"
    bench::kubectl create token "${sa}" --namespace="${namespace}" --duration="${duration}s" >"${out}"
}

# bench::api_forward makes the Results API reachable at https://localhost:${BENCH_API_PORT}
# for the harness and pins the client to the deployment under test. It always exports
# API_SERVER_ADDR and API_SERVER_NAME (the TLS SNI, keyed to the given namespace so it
# is correct even when BENCH_NAMESPACE overrides the default). On OpenShift it also
# starts a background port-forward, blocks until it is accepting connections, and
# registers an EXIT trap to tear it down. On kind the API is already published on
# localhost via the NodePort mapping, so no forward is started.
#
# Usage: bench::api_forward "${NAMESPACE}"
bench::api_forward() {
    local namespace="${1:-tekton-pipelines}"
    local port="${BENCH_API_PORT:-8080}"
    export API_SERVER_ADDR="https://localhost:${port}"
    export API_SERVER_NAME="tekton-results-api-service.${namespace}.svc.cluster.local"

    if [ "${BENCH_CLUSTER:-openshift}" = "kind" ]; then
        return 0
    fi

    if ! bench::kubectl wait deployment tekton-results-api --namespace="${namespace}" \
        --for=condition=available --timeout=120s; then
        echo "error: tekton-results-api is not available in ${namespace}; run 01-install.sh first" >&2
        return 1
    fi

    local log
    BENCH_API_FORWARD_DIR="$(mktemp -d "${TMPDIR:-/tmp}/tekton-results-api-forward.XXXXXX")"
    log="${BENCH_API_FORWARD_DIR}/port-forward.log"
    kubectl port-forward --namespace="${namespace}" svc/tekton-results-api-service "${port}:8080" >"${log}" 2>&1 &
    BENCH_API_FORWARD_PID=$!

    # shellcheck disable=SC2329  # invoked indirectly via the EXIT trap below
    bench::_api_forward_cleanup() {
        if [ -n "${BENCH_API_FORWARD_PID:-}" ]; then
            kill "${BENCH_API_FORWARD_PID}" 2>/dev/null || true
            wait "${BENCH_API_FORWARD_PID}" 2>/dev/null || true
            BENCH_API_FORWARD_PID=""
        fi
        if [ -n "${BENCH_API_FORWARD_DIR:-}" ]; then
            rm -rf -- "${BENCH_API_FORWARD_DIR}"
            BENCH_API_FORWARD_DIR=""
        fi
    }
    trap bench::_api_forward_cleanup EXIT

    local _
    for _ in {1..30}; do
        if grep -Fq 'Forwarding from' "${log}"; then
            return 0
        fi
        if ! kill -0 "${BENCH_API_FORWARD_PID}" 2>/dev/null; then
            cat "${log}" >&2
            echo "error: API port-forward to ${namespace}/tekton-results-api-service failed (is localhost:${port} already in use? set BENCH_API_PORT)" >&2
            return 1
        fi
        sleep 1
    done
    cat "${log}" >&2
    echo "error: timed out establishing API port-forward on localhost:${port}" >&2
    return 1
}

# bench::mem_to_bytes converts a Kubernetes memory quantity (e.g. 2Gi, 512Mi,
# 32633360Ki, or a bare byte count) to an integer number of bytes on stdout.
bench::mem_to_bytes() {
    local q="$1"
    case "${q}" in
    *Ki) echo $((${q%Ki} * 1024)) ;;
    *Mi) echo $((${q%Mi} * 1024 * 1024)) ;;
    *Gi) echo $((${q%Gi} * 1024 * 1024 * 1024)) ;;
    *Ti) echo $((${q%Ti} * 1024 * 1024 * 1024 * 1024)) ;;
    *[0-9]) echo "${q}" ;;
    *)
        echo "error: cannot parse memory quantity '${q}'" >&2
        return 1
        ;;
    esac
}

# bench::require_node_memory fails early (before the StatefulSet is patched) if the
# chosen database node cannot hold the Postgres memory request, turning the old
# silent "Pending forever" wedge into a clear, actionable error.
bench::require_node_memory() {
    local node="$1" required="$2"
    local allocatable req_bytes alloc_bytes
    allocatable="$(bench::kubectl get node "${node}" -o jsonpath='{.status.allocatable.memory}')"
    req_bytes="$(bench::mem_to_bytes "${required}")"
    alloc_bytes="$(bench::mem_to_bytes "${allocatable}")"
    if [ "${alloc_bytes}" -lt "${req_bytes}" ]; then
        echo "error: node ${node} has ${allocatable} allocatable memory, below the Postgres request of ${required}" >&2
        echo "       choose a larger database node or lower requests in postgres-patch.yaml" >&2
        return 1
    fi
}

# bench::clear_db_node_reservation removes the database label and taint (any effect)
# from every node we previously reserved, so re-running the installer starts clean
# and Postgres can be (re)scheduled without being repelled by a stale reservation.
bench::clear_db_node_reservation() {
    local nodes n
    nodes="$(bench::kubectl get nodes -l tekton-results.dev/role=database -o name 2>/dev/null || true)"
    for n in ${nodes}; do
        n="${n##*/}"
        echo "Clearing stale Postgres reservation from ${n}..."
        # A trailing "-" removes all taints with this key regardless of effect.
        bench::kubectl taint node "${n}" tekton-results.dev/role- >/dev/null 2>&1 || true
        bench::kubectl label node "${n}" tekton-results.dev/role- >/dev/null 2>&1 || true
    done
}

# bench::validate_json checks that a file parses as JSON, preferring python3, then
# jq, and finally a minimal structural check so the phase does not hard-depend on
# any single tool being installed on the operator's workstation.
bench::validate_json() {
    local file="$1"
    if command -v python3 >/dev/null 2>&1; then
        python3 -c "import json,sys; json.load(open(sys.argv[1]))" "${file}" >/dev/null 2>&1
    elif command -v jq >/dev/null 2>&1; then
        jq -e . "${file}" >/dev/null 2>&1
    else
        [ -s "${file}" ] && head -c1 "${file}" | grep -q '{'
    fi
}
