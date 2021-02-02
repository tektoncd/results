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

ROOT="$(git rev-parse --show-toplevel)"

for f in "taskrun.yaml" "pipelinerun.yaml"; do
    CONFIG="${ROOT}/test/e2e/${f}"
    echo "==========${CONFIG}=========="
    kubectl delete -f "${CONFIG}" || true
    kubectl apply -f "${CONFIG}" --record=false
    echo "Waiting for runs to complete..."
    kubectl wait -f "${CONFIG}" --for=condition=Succeeded

    # Try a few times to get the result, since we might query before the reconciler
    # picks it up.
    for n in $(seq 10); do
        result_id=$(kubectl get -f "${CONFIG}" -o json | jq -r '.metadata.annotations."results.tekton.dev/result"')
        if [[ "${result_id}" == "null" ]]; then
            echo "Attempt #${n}: Could not find 'results.tekton.dev/result' for ${CONFIG}"
            sleep 1
        fi
    done

    if [[ "${result_id}" == "null" ]]; then
        echo "Giving up."
        exit 1
    fi

    echo "Found result ${result_id}"
    echo "Success!"
done