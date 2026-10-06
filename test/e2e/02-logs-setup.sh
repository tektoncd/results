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

# Sets up in-cluster MinIO + Vector for blob log e2e tests.
# Called by e2e.sh before the blob log test suite.

set -o errexit
set -o pipefail
set -o nounset
set -x

ROOT="$(git rev-parse --show-toplevel)"

echo "Deploying in-cluster MinIO..."
kubectl apply -f "${ROOT}/test/e2e/blob-logs/minio.yaml"
kubectl wait deployment minio --namespace=tekton-pipelines \
    --for=condition=available --timeout=120s
kubectl wait job minio-bucket-init --namespace=tekton-pipelines \
    --for=condition=complete --timeout=120s

echo "Installing Vector via Helm..."
# Suppress set -x: Helm may echo values that contain credentials.
set +x
helm repo add vector https://helm.vector.dev 2>/dev/null || true
helm repo update
helm upgrade --install vector vector/vector \
    --namespace logging --create-namespace \
    --version 0.36.1 \
    --values "${ROOT}/test/e2e/blob-logs/vector-s3.yaml" \
    --wait --timeout 120s
set -x

echo "Applying blob plugin API config..."
kubectl apply -f "${ROOT}/test/e2e/blob-logs/vector-minio-config.yaml"

echo "Setting AWS credentials on API deployment..."
# The ConfigMap is read by viper as a file; AWS SDK env vars must be set
# on the container directly for s3blob to authenticate.
set +x
kubectl set env deployment/tekton-results-api -n tekton-pipelines \
    AWS_ACCESS_KEY_ID=tekton-results \
    AWS_SECRET_ACCESS_KEY=tekton-results-password
set -x

echo "Restarting API server and watcher to pick up new config..."
kubectl rollout restart deployment/tekton-results-api -n tekton-pipelines
kubectl rollout status deployment/tekton-results-api -n tekton-pipelines --timeout=120s
kubectl rollout restart deployment/tekton-results-watcher -n tekton-pipelines
kubectl rollout status deployment/tekton-results-watcher -n tekton-pipelines --timeout=120s

echo "Blob log setup complete."
