#!/bin/bash
# Copyright 2021 The Tekton Authors
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


# standard bash error handling
set -o errexit;
set -o pipefail;
set -o nounset;
# debug commands
set -x;

# cleanup on exit (useful for running locally)
cleanup() {
    kind delete cluster || true
}

trap cleanup EXIT

main() {
    export KO_DOCKER_REPO="kind.local"
    export KIND_CLUSTER_NAME="tekton-results"
    export SA_TOKEN_PATH=${SA_TOKEN_PATH:-"/tmp/tekton-results/tokens"}
    export SSL_CERT_PATH=${SSL_CERT_PATH:="/tmp/tekton-results/ssl"}

    REPO="$(git rev-parse --show-toplevel)"

    ${REPO}/test/e2e/00-setup.sh
    ${REPO}/test/e2e/01-install.sh

    # Build static binaries; otherwise go test complains.
    export CGO_ENABLED=0
    kubectl patch configmap tekton-results-config-logging -n tekton-pipelines --type='merge' -p='{ "data": {
        "zap-logger-config": "{\n  \"level\": \"debug\",\n  \"development\": false,\n  \"outputPaths\": [\"stdout\"],\n  \"errorOutputPaths\": [\"stderr\"],\n  \"encoding\": \"json\",\n  \"encoderConfig\": {\n    \"timeKey\": \"time\",\n    \"levelKey\": \"level\",\n    \"nameKey\": \"logger\",\n    \"callerKey\": \"caller\",\n    \"messageKey\": \"msg\",\n    \"stacktraceKey\": \"stacktrace\",\n    \"lineEnding\": \"\",\n    \"levelEncoder\": \"\",\n    \"timeEncoder\": \"iso8601\",\n    \"durationEncoder\": \"string\",\n    \"callerEncoder\": \"\"\n  }\n}",
        "loglevel.watcher": "debug"}
    }'
    kubectl get pod $(kubectl get pod -o=name -n tekton-pipelines | grep tekton-results-watcher | sed "s/^.\{4\}//") -n tekton-pipelines -o yaml
    go test -v -count=1 --tags=e2e $(go list --tags=e2e ${REPO}/test/e2e/... | grep -v /client)
    kubectl logs $(kubectl get pod -o=name -n tekton-pipelines | grep tekton-results-watcher | sed "s/^.\{4\}//") -n tekton-pipelines

    # Test GCS logging
    kubectl apply -f ${REPO}/test/e2e/gcs-emulator.yaml
    kubectl delete pod $(kubectl get pod -o=name -n tekton-pipelines | grep tekton-results-api | sed "s/^.\{4\}//") -n tekton-pipelines
    kubectl wait deployment "tekton-results-api" --namespace="tekton-pipelines" --for="condition=available" --timeout="120s"
    kubectl delete pod $(kubectl get pod -o=name -n tekton-pipelines | grep tekton-results-watcher | sed "s/^.\{4\}//") -n tekton-pipelines
    kubectl wait deployment "tekton-results-watcher" --namespace="tekton-pipelines" --for="condition=available" --timeout="120s"
    go test -v -count=1 --tags=e2e,gcs $(go list --tags=e2e ${REPO}/test/e2e/... | grep -v /client) -run TestGCSLog
}

main
