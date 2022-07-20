#! /bin/bash

set -ex

ROOT="$(git rev-parse --show-toplevel)"
# Default to short SHA if release version not set.
export RELEASE_VERSION=${RELEASE_VERSION:-"$(git rev-parse --short HEAD)"}

export KO_DOCKER_REPO=${KO_DOCKER_REPO:-"ko.local"}

RELEASE_DIR="${ROOT}/release"
# Apply templated values from environment.
sed -i "s/devel$/${RELEASE_VERSION}/g" ${RELEASE_DIR}/kustomization.yaml
sed -i "s/devel$/${RELEASE_VERSION}/g" ${ROOT}/config/config-info.yaml  

# Apply kustomiation + build images + generate yaml
kubectl kustomize ${RELEASE_DIR} | ko resolve --platform "linux/amd64,linux/ppc64le" -P -f - -t ${RELEASE_VERSION} > ${RELEASE_DIR}/release.yaml
