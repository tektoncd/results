# To build this, you need to use the repo root as the docker context:
#
# $ docker build -t <image tag> -f test/e2e/Dockerfile .
#
# To run this, you need to pass through the host docker socket + network:
#
# $ docker run -v /var/run/docker.sock:/var/run/docker.sock --network="host" <image tag>

FROM kind-runner

COPY . .

CMD ./test/e2e/runner.sh