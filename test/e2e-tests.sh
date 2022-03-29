#!/usr/bin/env bash

# Copyright 2022 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

source $(git rev-parse --show-toplevel)/test/vendor/github.com/tektoncd/plumbing/scripts/e2e-tests.sh

initialize $@

local failed=0

header "Installing Tekton Pipelines"
TEKTON_PIPELINE_CONFIG=${TEKTON_PIPELINE_CONFIG:-"https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml"}
kubectl apply --filename ${TEKTON_PIPELINE_CONFIG}

header "Generating DB secret"
# Don't fail if the secret isn't created - this can happen if the secret already exists.
kubectl create secret generic tekton-results-postgres --namespace="tekton-pipelines" --from-literal=POSTGRES_USER=postgres --from-literal=POSTGRES_PASSWORD=$(openssl rand -base64 20) || true

header "Generating TLS key pair"
set +e
  openssl req -x509 \
     -newkey rsa:4096 \
     -keyout "/tmp/tekton-results-key.pem" \
     -out "/tmp/tekton-results-cert.pem" \
     -days 365 \
     -nodes \
     -subj "/CN=tekton-results-api-service.tekton-pipelines.svc.cluster.local" \
     -addext "subjectAltName = DNS:tekton-results-api-service.tekton-pipelines.svc.cluster.local"
  if [ $? -ne 0 ] ; then
    echo "There was an error generating certificates"
    exit 1
  fi
set -e
kubectl create secret tls -n tekton-pipelines tekton-results-tls --cert="/tmp/tekton-results-cert.pem" --key="/tmp/tekton-results-key.pem" || true

header "Installing Tekton Results"
kubectl kustomize "${ROOT}/test/e2e/kustomize" | ko apply -f -
wait_until_pods_running "tekton-pipelines" || fail_test "Tekton Results did not come up"

header "Running e2e tests"
go_test_e2e -timeout=2m ./test/... || failed=1

(( failed )) && fail_test
success
