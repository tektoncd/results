#! /bin/bash

set -ex

ROOT="$(git rev-parse --show-toplevel)"
# Default to short SHA if release version not set.
export RELEASE_VERSION=${RELEASE_VERSION:-"$(git rev-parse --short HEAD)"}
export GITHUB_REPO=${GITHUB_REPO:-"https://github.com/tektoncd/results"}

export KO_DOCKER_REPO=${KO_DOCKER_REPO:-"ko.local"}

# Create a tag for ko
git tag ${RELEASE_VERSION}

RELEASE_DIR="${ROOT}/release"
# Apply templated values from environment.
sed -i "s/devel$/${RELEASE_VERSION}/g" ${RELEASE_DIR}/kustomization.yaml
sed -i "s/devel$/${RELEASE_VERSION}/g" ${ROOT}/config/base/config-info.yaml

# Apply kustomization + build images + generate yaml
kubectl kustomize ${RELEASE_DIR} | ko resolve \
    --image-label=org.opencontainers.image.source=${GITHUB_REPO} \
    --platform "linux/amd64,linux/arm,linux/arm64,linux/ppc64le,linux/s390x" \
    ${KO_EXTRA_ARGS} -f - -t ${RELEASE_VERSION} > ${RELEASE_DIR}/release_base.yaml

cp ${RELEASE_DIR}/release_base.yaml ${RELEASE_DIR}/release.yaml
kubectl kustomize ${RELEASE_DIR}/localdb >> ${RELEASE_DIR}/release.yaml
