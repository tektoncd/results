# Results E2E tests

## Quickstart

```sh
$ ./00-setup.sh
$ ./01-install.sh
$ go test --tags=e2e .
```

## Dependencies

- go (>= go1.19)
- git
- kubectl
- ko (>= v0.6.2)
- kind
- jq

## E2E Test Environment Variables

The e2e tests use environment variables to modify default values, such as the server name, server address, certificate
path, etc.The scripts set some of the variables, and you can set other variables to run e2e tests manually.

| Environment variable | Description                                                 | Default                                                       |
|----------------------|-------------------------------------------------------------|---------------------------------------------------------------| 
| API_SERVER_ADDR      | The address on which results API server is listening        | https://localhost:8080                                        |
| API_SERVER_NAME      | Common Name of the server as defined in the SSL certificate | tekton-results-api-service.tekton-pipelines.svc.cluster.local |
| CERT_FILE_NAME       | Name of the certificate file                                | tekton-results-cert.pem                                       |
| SSL_CERT_PATH        | Path of the directory containing SSL certificates           | /tmp/tekton-results/ssl                                       |
| SA_TOKEN_PATH        | Path of the directory containing service account tokens     | /tmp/tekton-results/tokens                                    |

## Scripts

This folder contains several scripts, useful for testing e2e workflows:

### `00-setup.sh`

Sets up a local kind cluster, and configures your local kubectl context to use
this environment.

| Environment variable | Description              | Default                                                                                      |
|----------------------|--------------------------|----------------------------------------------------------------------------------------------|
| KIND_CLUSTER_NAME    | KIND cluster name to use | tekton-results                                                                               |
| KIND_IMAGE           | KIND node image to use   | kindest/node:v1.25.3@sha256:f52781bc0d7a19fb6c405c2af83abfeb311f130707a0e219175677e366cc45d1 |

### `01-install.sh`

Installs Tekton Pipelines and Results components. Results is always installed
from the local repo.

All components are installed to the current kubectl context
(`kubectl config current-context`).

This can safely be ran multiple times, and should be ran anytime a change is
made to Results components.

| Environment variable   | Description                                                                   | Default                                                                     |
| ---------------------- | ----------------------------------------------------------------------------- | --------------------------------------------------------------------------- |
| KO_DOCKER_REPO         | Docker repository to use for ko                                               | kind.local                                                                  |
| TEKTON_PIPELINE_CONFIG | Tekton Pipelines config source (anything `kubectl apply -f` compatible)       | https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml |
| KIND_CLUSTER_NAME      | Name of the kind cluster for testing                                          | `tekton-results`                                                            |
| SA_TOKEN_PATH          | Path to store the service account tokens used for testing                     | `/tmp/tekton-results/tokens`                                                |
| SSL_CERT_PATH          | Path to store the SSL certificate used to secure the gRPC endpoint            | `/tmp/tekton-results/ssl`                                                   |
| SSL_INCLUDE_LOCALHOST  | Include "localhost" as an alternate DNS name in the generated SSL certificate | "false"                                                                     |

## Running the tests

Once you have configured your local client, you can run the tests by running:

```sh
$ go test --tags=e2e .
```
