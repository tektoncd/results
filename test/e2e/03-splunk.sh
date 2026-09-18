#!/bin/bash
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

set -euo pipefail
ROOT="$(git rev-parse --show-toplevel)"
VECTOR_CHART_VERSION="${VECTOR_CHART_VERSION:-0.40.0}"
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-tekton-results}"
SPLUNK_IMAGE="${SPLUNK_IMAGE:-splunk/splunk:9.2}"

# Splunk's Hub tag is a multi-arch *list* with only an amd64 image.
docker pull --platform linux/amd64 "${SPLUNK_IMAGE}"
ARCHIVE="$(mktemp /tmp/splunk.XXXXXX.tar)"
docker image save --platform linux/amd64 -o "${ARCHIVE}" "${SPLUNK_IMAGE}"
kind load image-archive "${ARCHIVE}" --name "${KIND_CLUSTER_NAME}"
rm -f "${ARCHIVE}"

kubectl apply -f "${ROOT}/test/e2e/splunk/splunk.yaml"
# Recreate so /tmp/defaults from the ConfigMap is applied on a fresh Splunk.
kubectl delete pod -n logging -l app=splunk --ignore-not-found --wait=true
kubectl rollout status statefulset/splunk -n logging --timeout=10m

bash "${ROOT}/test/e2e/splunk/create-search-token.sh"

helm repo add vector https://helm.vector.dev
helm repo update
helm upgrade --install vector vector/vector \
  --namespace logging --create-namespace \
  --version "${VECTOR_CHART_VERSION}" \
  --values "${ROOT}/test/e2e/splunk/vector.yaml" \
  --wait --timeout 5m

kubectl apply -f "${ROOT}/test/e2e/splunk/api-config.yaml"
kubectl rollout restart deployment tekton-results-api -n tekton-pipelines
kubectl rollout status deployment tekton-results-api -n tekton-pipelines --timeout=180s
