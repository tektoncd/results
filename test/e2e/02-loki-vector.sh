#!/bin/bash
# Copyright 2024 The Tekton Authors
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

# shellcheck disable=SC2181 # To ignore long command exit code check

set -e

ROOT="$(git rev-parse --show-toplevel)"

# Install Grafana Loki
helm repo add grafana https://grafana.github.io/helm-charts
helm repo update
helm upgrade --install loki grafana/loki --namespace logging --create-namespace --values ${ROOT}/test/e2e/loki_vector/loki.yaml

# Install Vector
helm repo add vector https://helm.vector.dev
helm repo update
helm upgrade --install vector vector/vector --namespace logging --values ${ROOT}/test/e2e/loki_vector/vector.yaml

# Update Results API ConfigMap   
kubectl apply -f ${ROOT}/test/e2e/loki_vector/loki-vector-api-config.yaml

# Rollout Restart Results API Deployment
kubectl rollout restart deployment tekton-results-api -n tekton-pipelines

# Update Results Watcher Deployment Args
kubectl patch deployment \
  tekton-results-watcher \
  --namespace tekton-pipelines \
  --type='json' \
  -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/args", "value": [
  "-api_addr",
  "$(TEKTON_RESULTS_API_SERVICE)",
  "-auth_mode",
  "$(AUTH_MODE)",
  "-store_event",
  "true"
]}]'
