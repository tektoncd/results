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

# Sets up fake-gcs-server for GCS blob log e2e tests.
#
# Vector is NOT used for the GCS leg. Its gcp_cloud_storage sink speaks the
# GCS XML API, but fake-gcs-server only implements the JSON API. Instead,
# test data is seeded directly via the JSON API from the CI host
# (port-forwarded). The test then proves the gcsblob read path works
# end-to-end.
#
# Called by e2e.sh before the gcs_blob test suite.

set -o errexit
set -o pipefail
set -o nounset
set -x

ROOT="$(git rev-parse --show-toplevel)"

echo "Deploying fake-gcs-server emulator..."
kubectl apply -f "${ROOT}/test/e2e/blob-logs/gcs-emulator-deployment.yaml"
kubectl wait deployment gcs-emulator --namespace=tekton-pipelines \
    --for=condition=available --timeout=120s

echo "Seeding test data via JSON API (port-forward from host)..."
# Port-forward the emulator to the host and seed via the runner's own curl.
# This avoids pulling a separate curl image inside the kind cluster, which
# is slow on GitHub Actions runners due to Docker Hub rate limits.
kubectl port-forward -n tekton-pipelines svc/gcs-emulator 19000:9000 &
PF_PID=$!
trap "kill ${PF_PID} 2>/dev/null || true" EXIT

EMULATOR="http://localhost:19000"

# Wait for the port-forward + emulator to be reachable.
for i in $(seq 1 30); do
    if curl -s --connect-timeout 2 --max-time 5 -o /dev/null "${EMULATOR}/storage/v1/b?project=test" 2>/dev/null; then
        break
    fi
    if [ "$i" -eq 30 ]; then
        echo "ERROR: GCS emulator not reachable via port-forward after 30s"
        exit 1
    fi
    sleep 1
done

echo "Creating bucket tekton-logs..."
HTTP_CODE=$(curl -s --connect-timeout 10 --max-time 30 -o /dev/null -w "%{http_code}" -X POST \
    "${EMULATOR}/storage/v1/b?project=test" \
    -H "Content-Type: application/json" \
    -d '{"name":"tekton-logs"}')
if [ "$HTTP_CODE" != "200" ] && [ "$HTTP_CODE" != "409" ]; then
    echo "ERROR: bucket create returned HTTP ${HTTP_CODE}"
    exit 1
fi

echo "Seeding test objects..."
curl -sS --connect-timeout 10 --max-time 30 --fail -X POST \
    "${EMULATOR}/upload/storage/v1/b/tekton-logs/o?uploadType=media&name=logs/default/gcs-blob-test/gcs-blob-record/step-test" \
    -H "Content-Type: text/plain" \
    -d "GCS_BLOB_MARKER_TEST"
echo ""

kill ${PF_PID} 2>/dev/null || true
# Remove the EXIT trap so it doesn't fire again during later steps.
trap - EXIT

echo "Applying GCS blob plugin API config..."
kubectl apply -f "${ROOT}/test/e2e/blob-logs/gcs-emulator-blob-config.yaml"

echo "Setting STORAGE_EMULATOR_HOST on API deployment..."
# gcsblob reads os.Getenv("STORAGE_EMULATOR_HOST"), not config.
# Only the legacy GCS path copies config to env; the plugin path does not.
kubectl set env deployment/tekton-results-api -n tekton-pipelines \
    STORAGE_EMULATOR_HOST=gcs-emulator.tekton-pipelines.svc.cluster.local:9000
# Remove any leftover S3 creds from the previous suite.
kubectl set env deployment/tekton-results-api -n tekton-pipelines \
    AWS_ACCESS_KEY_ID- AWS_SECRET_ACCESS_KEY-

echo "Restarting API server and watcher to pick up new config..."
kubectl rollout restart deployment/tekton-results-api -n tekton-pipelines
kubectl rollout status deployment/tekton-results-api -n tekton-pipelines --timeout=120s
kubectl rollout restart deployment/tekton-results-watcher -n tekton-pipelines
kubectl rollout status deployment/tekton-results-watcher -n tekton-pipelines --timeout=120s

echo "GCS blob log setup complete."
