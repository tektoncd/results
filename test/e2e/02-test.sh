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
TASKRUN="${ROOT}/test/e2e/taskrun.yaml"

kubectl delete -f "${TASKRUN}" || true
kubectl apply -f "${TASKRUN}"
echo "Waiting for TaskRun to complete..."
kubectl wait -f "${TASKRUN}" --for=condition=Succeeded

# Try a few times to get the result, since we might query before the reconciler
# picks it up.
for n in $(seq 10); do
    result_id=$(kubectl get -f "${TASKRUN}" -o json | jq -r '.metadata.annotations."results.tekton.dev/result"')
    if [[ "${result_id}" == "null" ]]; then
        echo "Attempt #${n}: Could not find 'results.tekton.dev/result' for ${TASKRUN}"
        sleep 1
    fi
done

if [[ "${result_id}" == "null" ]]; then
    echo "Giving up."
    exit 1
fi

echo "Found result ${result_id}"
echo "Success!"