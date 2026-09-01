#!/bin/bash
# Copyright 2020 The Tekton Authors
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

set -e

MODE="${1:-standard}" # "standard" or "ha"

export KO_DOCKER_REPO=${KO_DOCKER_REPO:-"kind.local"}
export KIND_CLUSTER_NAME=${KIND_CLUSTER_NAME:-"tekton-results"}
export SA_TOKEN_PATH=${SA_TOKEN_PATH:-"/tmp/tekton-results/tokens"}
export SSL_CERT_PATH=${SSL_CERT_PATH:="/tmp/tekton-results/ssl"}

ROOT="$(git rev-parse --show-toplevel)"

echo "Installing Tekton Pipelines..."
TEKTON_PIPELINE_CONFIG=${TEKTON_PIPELINE_CONFIG:-"https://infra.tekton.dev/tekton-releases/pipeline/latest/release.yaml"}
kubectl apply --filename "${TEKTON_PIPELINE_CONFIG}"
kubectl wait --for=condition=ready pod -l app=tekton-pipelines-controller -n tekton-pipelines --timeout=120s
kubectl wait --for=condition=ready pod -l app=tekton-pipelines-webhook -n tekton-pipelines --timeout=120s

echo "Generating DB secret..."
# Don't fail if the secret isn't created - this can happen if the secret already exists.
kubectl create secret generic tekton-results-postgres --namespace="tekton-pipelines" --from-literal=POSTGRES_USER=postgres --from-literal=POSTGRES_PASSWORD="$(openssl rand -base64 20)" || true

echo "Generating TLS key pair..."
mkdir -p "${SSL_CERT_PATH}"

# The headless service SAN is only needed for the HA deployment, where the
# watcher connects to the API via DNS-based load balancing over the headless
# service. generate-tls-cert.sh includes it by default, so opt out for the
# standard, single-replica install.
if [ "$MODE" = "ha" ]; then
    export EXTRA_SANS="DNS:tekton-results-api-service-headless.tekton-pipelines.svc.cluster.local"
else
    export EXTRA_SANS=""
fi
export OUTPUT_DIR="${SSL_CERT_PATH}"
export CERT_FILE_NAME="tekton-results-cert.pem"
export KEY_FILE_NAME="tekton-results-key.pem"
"${ROOT}/config/components/horizontal-scaling/generate-tls-cert.sh"

if [ "$MODE" = "ha" ]; then
    echo "Installing Tekton Results with HA configuration..."
    kustomize_dir="${ROOT}/test/e2e/kustomize-ha"
else
    echo "Installing Tekton Results..."
    kustomize_dir="${ROOT}/test/e2e/kustomize"
fi
extra_ko_params="linux/$(go env GOARCH)"
kubectl kustomize "${kustomize_dir}" | ko apply --platform="$extra_ko_params" -f -

echo "Fetching access tokens..."
mkdir -p "${SA_TOKEN_PATH}"
service_accounts=(all-namespaces-read-access single-namespace-read-access all-namespaces-admin-access all-namespaces-impersonate-access)
for service_account in "${service_accounts[@]}"; do
    kubectl create token "$service_account" > "${SA_TOKEN_PATH}"/"$service_account"
    echo "Created ${SA_TOKEN_PATH}/$service_account"
done

if [ "$MODE" = "ha" ]; then
    echo "Waiting for Tekton Results pods..."
    # The watcher and API run with multiple replicas in HA mode, so wait on
    # the pod labels rather than a single Deployment/StatefulSet rollout.
    kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=tekton-results-api -n tekton-pipelines --timeout=300s
    kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=tekton-results-watcher -n tekton-pipelines --timeout=300s
    kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=tekton-results-postgres -n tekton-pipelines --timeout=120s

    echo ""
    echo "Installation complete!"
    echo ""
    kubectl get pods -n tekton-pipelines | grep tekton-results
    echo ""
    echo "Run E2E tests: go test -v --tags=e2e,e2e_ha -run TestHorizontalScaling ./test/e2e/..."
else
    echo "Waiting for deployments to be ready..."
    kubectl wait pod "tekton-results-postgres-0" --namespace="tekton-pipelines" --for="condition=Ready" --timeout="120s"
    kubectl wait deployment "tekton-results-api" --namespace="tekton-pipelines" --for="condition=available" --timeout="120s"
    kubectl wait deployment "tekton-results-watcher" --namespace="tekton-pipelines" --for="condition=available" --timeout="120s"
fi
