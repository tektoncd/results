#! /bin/bash

set -e

ROOT="$(git rev-parse --show-toplevel)"
# Default to short SHA if release version not set.
RELEASE_VERSION=${RELEASE_VERSION:-"$(git rev-parse --short HEAD)"}

export KO_DOCKER_REPO=${KO_DOCKER_REPO:-"ko.local"}

ko resolve -P -f ${ROOT}/config -t ${RELEASE_VERSION} | sed "s/devel/${RELEASE_VERSION}/g" > ${ROOT}/release/release.yaml