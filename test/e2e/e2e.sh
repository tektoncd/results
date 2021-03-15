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

    REPO="$(git rev-parse --show-toplevel)"

    ${REPO}/test/e2e/00-setup.sh
    ${REPO}/test/e2e/01-install.sh

    # Build static binaries; otherwise go test complains.
    export CGO_ENABLED=0
    go test --tags=e2e ${REPO}/test/e2e/...
}

main