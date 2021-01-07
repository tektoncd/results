#!/bin/bash

set -euxo pipefail

# Starts a local docker registry and connects it to kind.
#
# Required for e2e tests to work.
reg_name='kind-registry'
reg_port='5000'
docker inspect "${reg_name}" &>/dev/null || (
  # The container doesn't exist.
  docker run \
    -d --restart=always -p "${reg_port}:5000" --name "${reg_name}" \
    registry:2
)

# The container exists, but might not be running.
# It's safe to run this even if the container is already running.
docker start "${reg_name}"

# Ensure kind v0.9.0 is installed.
# Dear future people: Feel free to upgrade this as new versions are released.
# Note that upgrading the kind version will require updating the image versions:
# https://github.com/kubernetes-sigs/kind/releases
kind &> /dev/null || (
  echo "Kind is not installed. Install v0.10.0."
  echo "https://kind.sigs.k8s.io/docs/user/quick-start/"
  exit 1
)
kind version | grep v0.10.0 || (
  echo "Using unsupported kind version. Install v0.9.0."
  echo "https://kind.sigs.k8s.io/docs/user/quick-start/"
  exit 1
)

# Check if the "kind" docker network exists.
docker network inspect "kind" >/dev/null || (
  # kind doesn't create the docker network until it has been used to create a
  # cluster.
  kind create cluster
  kind delete cluster
)

# Connect the registry to the cluster network if it isn't already.
docker network inspect kind | grep "${reg_name}" || \
  docker network connect "kind" "${reg_name}"

exec $@