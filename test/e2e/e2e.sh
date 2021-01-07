#! /bin/bash

set -x

WORKDIR=${WORKDIR:-"$(git rev-parse --show-toplevel)"}
cd ${WORKDIR}

TAG="results-e2e:$(git rev-parse --short HEAD)"

docker build -t "kind-runner" ./test/e2e/kind-runner
docker build -t "${TAG}" -f test/e2e/Dockerfile .
docker run -v /var/run/docker.sock:/var/run/docker.sock --network="host" "${TAG}"