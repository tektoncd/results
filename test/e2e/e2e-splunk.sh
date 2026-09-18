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

set -o errexit; set -o pipefail; set -o nounset; set -x
cleanup() { kind delete cluster || true; }
trap cleanup EXIT
export KO_DOCKER_REPO="kind.local"
export KIND_CLUSTER_NAME="tekton-results"
export SA_TOKEN_PATH=${SA_TOKEN_PATH:-"/tmp/tekton-results/tokens"}
export SSL_CERT_PATH=${SSL_CERT_PATH:="/tmp/tekton-results/ssl"}
export SSL_INCLUDE_LOCALHOST=true
REPO="$(git rev-parse --show-toplevel)"
"${REPO}/test/e2e/00-setup.sh"
"${REPO}/test/e2e/01-install.sh"
"${REPO}/test/e2e/03-splunk.sh"
export CGO_ENABLED=0
go test -v -count=1 --tags=e2e,splunk $(go list --tags=e2e ${REPO}/test/e2e/... | grep -v /client) -timeout 20m