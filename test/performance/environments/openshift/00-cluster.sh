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
# Phase 0 — select and validate the target cluster for a tier-2 benchmark.
#
#   ./00-cluster.sh --cluster openshift   # (default) attach to a booked cluster
#   ./00-cluster.sh --cluster kind        # local dry run of the tier-2 pipeline
#
# openshift: the ephemeral multi-node cluster is provisioned externally; this
#            script validates that $KUBECONFIG points at a reachable cluster and
#            runs SAFETY CHECKS to prevent accidentally running destructive benchmark
#            operations on a production/working cluster. It refuses if it detects:
#            - Tekton Results installed in openshift-pipelines (production namespace)
#            - Production-named namespaces (production, prod-*, staging)
#            - Non-ephemeral cluster domain patterns (not *.openshiftapps.com)
#            Override with BENCH_ALLOW_PRODUCTION=yes only for disposable clusters.
# kind:      delegates to the tier-1 kind bootstrap so the full pipeline can be
#            rehearsed locally. Results are dev-only, NOT a tier-2 baseline.

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(git rev-parse --show-toplevel)"
# shellcheck source=test/performance/environments/openshift/common.sh
source "${HERE}/common.sh"

bench::resolve_cluster "$@"
bench::start_timer
bench::phase "phase 0: cluster (${CLUSTER})"

if [ "${CLUSTER}" = "kind" ]; then
    echo "kind mode: dry run only — results are NOT a tier-2 baseline."
    if kubectl cluster-info >/dev/null 2>&1; then
        echo "Reusing the current kube context."
    else
        echo "No reachable cluster; standing up the tier-1 kind cluster..."
        "${ROOT}/test/performance/environments/kind/00-kind-up.sh"
    fi
else
    echo "openshift mode: validating the booked ephemeral cluster in \$KUBECONFIG."
    if ! kubectl cluster-info >/dev/null 2>&1; then
        echo "error: no reachable cluster; point \$KUBECONFIG at the booked OpenShift cluster" >&2
        exit 1
    fi
    # clusterversion (config.openshift.io) exists only on OpenShift; this catches a
    # kind/vanilla-k8s context accidentally left in $KUBECONFIG, which would later
    # fail deep in 01-install.sh (SCC/registry steps) with a confusing error.
    if ! kubectl get clusterversion >/dev/null 2>&1; then
        echo "error: cluster in \$KUBECONFIG is not OpenShift; use --cluster kind for a local dry run" >&2
        exit 1
    fi
    nodes="$(kubectl get nodes --no-headers 2>/dev/null | wc -l | tr -d ' ')"
    echo "Cluster reachable with ${nodes} node(s)."
    if [ "${nodes}" -lt 3 ]; then
        echo "warning: expected a multi-node cluster for a representative tier-2 run (got ${nodes})" >&2
    fi

    # Safety check: refuse to run on production/working clusters. Benchmark operations
    # are destructive (database resets, Postgres tuning, high-concurrency load) and
    # must only target ephemeral throwaway clusters. Detect production by checking for:
    # 1. Results installed in openshift-pipelines (production namespace)
    # 2. Production-named namespaces
    # 3. Non-ephemeral cluster domain patterns
    echo "Checking cluster is safe for destructive benchmark operations..."

    # Check 1: Results in production namespace (openshift-pipelines)
    if kubectl get deployment tekton-results-api -n openshift-pipelines >/dev/null 2>&1; then
        echo "error: this cluster has Tekton Results installed in openshift-pipelines (production namespace)" >&2
        echo "       Benchmark scripts are DESTRUCTIVE and must only run on ephemeral throwaway clusters." >&2
        echo "       If this is genuinely a disposable cluster, set BENCH_ALLOW_PRODUCTION=yes to override." >&2
        if [ "${BENCH_ALLOW_PRODUCTION:-no}" != "yes" ]; then
            exit 1
        fi
        echo "warning: BENCH_ALLOW_PRODUCTION=yes override active — proceeding on a cluster with production markers" >&2
    fi

    # Check 2: Production namespace patterns
    prod_namespaces=$(kubectl get namespaces -o name 2>/dev/null | grep -iE 'production|prod-|staging' | head -3)
    if [ -n "${prod_namespaces}" ]; then
        echo "warning: cluster has production-named namespaces:" >&2
        echo "${prod_namespaces}" | while IFS= read -r ns; do echo "         ${ns}"; done >&2
        echo "       This may be a working cluster. Benchmark operations are DESTRUCTIVE." >&2
        if [ "${BENCH_ALLOW_PRODUCTION:-no}" != "yes" ]; then
            echo "       Set BENCH_ALLOW_PRODUCTION=yes to override if this is genuinely disposable." >&2
            exit 1
        fi
        echo "       BENCH_ALLOW_PRODUCTION=yes override active — proceeding" >&2
    fi

    # Check 3: Cluster domain (ephemeral clusters use *.openshiftapps.com)
    cluster_server=$(kubectl cluster-info 2>/dev/null | grep -oP 'https://[^:]+' | head -1)
    if [ -n "${cluster_server}" ] && ! echo "${cluster_server}" | grep -qE 'openshiftapps\.com|ephemeral|tmp-|test-'; then
        echo "warning: cluster domain does not match ephemeral patterns: ${cluster_server}" >&2
        echo "       Ephemeral clusters typically use *.openshiftapps.com or contain 'ephemeral'/'tmp-'/'test-'" >&2
        if [ "${BENCH_ALLOW_PRODUCTION:-no}" != "yes" ]; then
            echo "       Set BENCH_ALLOW_PRODUCTION=yes to override if this is genuinely a benchmark cluster." >&2
            exit 1
        fi
        echo "       BENCH_ALLOW_PRODUCTION=yes override active — proceeding" >&2
    fi
    # A freshly booked ephemeral cluster often reports reachable while its nodes are
    # still NotReady, so nothing can schedule. Gate here so a boot-in-progress fails
    # as a clear phase-0 error instead of a confusing "pod is not running" deep in
    # 01-install.sh (e.g. the registry port-forward against a Pending pod).
    echo "Waiting for all nodes to be Ready..."
    if ! kubectl wait --for=condition=Ready nodes --all --timeout=180s >/dev/null 2>&1; then
        echo "error: not all nodes are Ready; the cluster may still be booting — wait and retry" >&2
        exit 1
    fi
fi

cat <<EOF

Phase 0 complete (cluster=${CLUSTER}).

Thread the same choice through the remaining phases, e.g.:

  export BENCH_CLUSTER=${CLUSTER}
  ./01-install.sh
  ./02-load.sh
  ./03-benchmark.sh

or pass --cluster ${CLUSTER} to each phase explicitly.
EOF
