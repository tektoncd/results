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

run_db_tests() {
    REPO="$1"
    local LOCAL_PG_PORT=15432

    echo "Starting Postgres port-forward on localhost:${LOCAL_PG_PORT}..."
    kubectl port-forward svc/tekton-results-postgres-service "${LOCAL_PG_PORT}":5432 -n tekton-pipelines &
    PF_PID=$!

    # Wait until the forwarded port actually accepts connections instead of
    # a blind sleep. Bail after 30 seconds.
    for i in $(seq 1 30); do
        if bash -c "echo >/dev/tcp/localhost/${LOCAL_PG_PORT}" 2>/dev/null; then
            break
        fi
        if [ "$i" -eq 30 ]; then
            echo "ERROR: port-forward to Postgres did not become ready"
            kill "${PF_PID}" 2>/dev/null || true
            return 1
        fi
        sleep 1
    done

    # Build DB_URL once; suppress set -x so the password stays out of CI logs.
    set +x
    PGPASS=$(kubectl get secret tekton-results-postgres -n tekton-pipelines \
        -o jsonpath='{.data.POSTGRES_PASSWORD}' | base64 -d)
    DB_URL="host=localhost port=${LOCAL_PG_PORT} user=postgres password=${PGPASS} dbname=tekton-results sslmode=disable"

    echo "Running Postgres DB layer e2e tests..."
    local test_rc=0
    DB_URL="${DB_URL}" go test -v -count=1 -tags=e2e "${REPO}/test/e2e/db/..." || test_rc=$?
    set -x

    kill "${PF_PID}" 2>/dev/null || true

    if [ "${test_rc}" -ne 0 ]; then
        echo "ERROR: DB layer e2e tests failed (exit ${test_rc})"
        return "${test_rc}"
    fi
}

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
    go test -v -count=1 --tags=e2e $(go list --tags=e2e ${REPO}/test/e2e/... | grep -v /client | grep -v /db)
    kubectl logs $(kubectl get pod -o=name -n tekton-pipelines | grep tekton-results-watcher | sed "s/^.\{4\}//") -n tekton-pipelines

    # Postgres DB layer tests
    run_db_tests "${REPO}"

    # Test GCS logging
    kubectl apply -f ${REPO}/test/e2e/gcs-emulator.yaml
    kubectl delete pod $(kubectl get pod -o=name -n tekton-pipelines | grep tekton-results-api | sed "s/^.\{4\}//") -n tekton-pipelines
    kubectl wait deployment "tekton-results-api" --namespace="tekton-pipelines" --for="condition=available" --timeout="120s"
    kubectl delete pod $(kubectl get pod -o=name -n tekton-pipelines | grep tekton-results-watcher | sed "s/^.\{4\}//") -n tekton-pipelines
    kubectl wait deployment "tekton-results-watcher" --namespace="tekton-pipelines" --for="condition=available" --timeout="120s"
    go test -v -count=1 --tags=e2e,gcs $(go list --tags=e2e ${REPO}/test/e2e/... | grep -v /client | grep -v /db) -run TestGCSLog
}

main
