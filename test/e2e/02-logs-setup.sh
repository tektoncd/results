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

# shellcheck disable=SC2181 # To ignore long command exit code check

set -e

ROOT="$(git rev-parse --show-toplevel)"

curl https://dl.min.io/client/mc/release/linux-amd64/mc \
  --create-dirs \
  -o $HOME/minio-binaries/mc

chmod +x $HOME/minio-binaries/mc
export PATH=$PATH:$HOME/minio-binaries/

mc alias set myPlayMinio https://play.min.io:9000  Q3AM3UQ867SPQQA43P2F zuf+tfteSlswRu7BJ86wekitnifILbZam1KYY3TG

mc mb myPlayMinio/tekton-logs

helm upgrade --install vector vector/vector --namespace logging --values ${ROOT}/test/e2e/blob-logs/vector-s3.yaml

kubectl apply -f ${ROOT}/test/e2e/blob-logs/vector-minio-config.yaml
kubectl delete pod $(kubectl get pod -o=name -n tekton-pipelines | grep tekton-results-api | sed "s/^.\{4\}//") -n tekton-pipelines
kubectl wait deployment "tekton-results-api" --namespace="tekton-pipelines" --for="condition=available" --timeout="120s"
kubectl delete pod $(kubectl get pod -o=name -n tekton-pipelines | grep tekton-results-watcher | sed "s/^.\{4\}//") -n tekton-pipelines
kubectl wait deployment "tekton-results-watcher" --namespace="tekton-pipelines" --for="condition=available" --timeout="120s"
