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

export KO_DOCKER_REPO=${KO_DOCKER_REPO:-"kind.local"}
export KIND_CLUSTER_NAME=${KIND_CLUSTER_NAME:-"tekton-results"}

ROOT="$(git rev-parse --show-toplevel)"

echo "Installing Tekton Pipelines..."
TEKTON_PIPELINE_CONFIG=${TEKTON_PIPELINE_CONFIG:-"https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml"}
kubectl apply --filename ${TEKTON_PIPELINE_CONFIG}

echo "Generating DB secret..."
# Don't fail if the secret isn't created - this can happen if the secret already exists.
kubectl create secret generic tekton-results-postgres --namespace="tekton-pipelines" --from-literal=POSTGRES_USER=postgres --from-literal=POSTGRES_PASSWORD=$(openssl rand -base64 20) || true

echo "Generating TLS key pair..."
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
    # LibreSSL didn't support the -addext flag until version 3.1.0 but
    # version 2.8.3 ships with MacOS Big Sur. So let's try a different way...
    echo "Falling back to legacy libressl cert generation"
    openssl req -x509 \
      -verbose \
      -config <(cat /etc/ssl/openssl.cnf <(printf "[SAN]\nsubjectAltName = DNS:tekton-results-api-service.tekton-pipelines.svc.cluster.local")) \
      -extensions SAN \
      -newkey rsa:4096 \
      -keyout "/tmp/tekton-results-key.pem" \
      -out "/tmp/tekton-results-cert.pem" \
      -days 365 \
      -nodes \
      -subj "/CN=tekton-results-api-service.tekton-pipelines.svc.cluster.local"

    if [ $? -ne 0 ] ; then
      echo "There was an error generating certificates"
      exit 1
    fi
  fi
set -e
kubectl create secret tls -n tekton-pipelines tekton-results-tls --cert="/tmp/tekton-results-cert.pem" --key="/tmp/tekton-results-key.pem" || true

echo "Installing Tekton Results..."
kubectl kustomize "${ROOT}/test/e2e/kustomize" | ko apply -f -

echo "Waiting for deployments to be ready..."
kubectl wait deployment "tekton-results-api" --namespace="tekton-pipelines" --for="condition=available" --timeout="60s"
kubectl wait deployment "tekton-results-watcher" --namespace="tekton-pipelines" --for="condition=available" --timeout="60s"
