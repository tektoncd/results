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

Accepts an optional mode argument: `./01-install.sh` (standard, default) or
`./01-install.sh ha` (horizontal scaling configuration, see below).

| Environment variable   | Description                                                                   | Default                                                                     |
| ---------------------- | ----------------------------------------------------------------------------- | --------------------------------------------------------------------------- |
| KO_DOCKER_REPO         | Docker repository to use for ko                                               | kind.local                                                                  |
| TEKTON_PIPELINE_CONFIG | Tekton Pipelines config source (anything `kubectl apply -f` compatible)       | https://infra.tekton.dev/tekton-releases/pipeline/latest/release.yaml |
| KIND_CLUSTER_NAME      | Name of the kind cluster for testing                                          | `tekton-results`                                                            |
| SA_TOKEN_PATH          | Path to store the service account tokens used for testing                     | `/tmp/tekton-results/tokens`                                                |
| SSL_CERT_PATH          | Path to store the SSL certificate used to secure the gRPC endpoint            | `/tmp/tekton-results/ssl`                                                   |
| SSL_INCLUDE_LOCALHOST  | Include "localhost" as an alternate DNS name in the generated SSL certificate | "false"                                                                     |

## Running the tests

Once you have configured your local client, you can run the tests by running:

```sh
$ go test --tags=e2e .
```
### HA E2E Tests

The E2E test suite verifies the three correctness guarantees using a real Kubernetes cluster with the HA configuration deployed.

**Test file:** `test/e2e/e2e_ha_test.go`
**Prerequisites:**
1. HA configuration deployed via `test/e2e/01-install.sh ha`
2. Tekton Pipelines installed
3. Service account tokens extracted for API authentication

**Run tests:**

```bash
cd test/e2e
./01-install.sh ha
go test -v --tags=e2e,e2e_ha -run TestHorizontalScaling .
```

### Test Structure

**TestHorizontalScaling** contains four subtests:

**1. VerifyPods** (precondition)

Polls pod status and asserts:
- 3 API pods are Ready
- 3 watcher pods are Ready
- 1 Postgres pod is Ready

Fails fast if the deployment is unhealthy.

**2. NoDuplicates**

Creates 9 TaskRuns and waits for Result/Record annotations.

**API-side check:**

For each TaskRun's Result, calls ListRecords and counts TaskRun-type records (filters by `Data.Type` to exclude log/event records). Asserts exactly 1 TaskRun record per Result.

**Watcher-side check:**

Reads logs from all watcher pods via Kubernetes Logs API. Searches for log entries containing `"knative.dev/key":"default/<taskrun-name>"`. Asserts each TaskRun appears in exactly one watcher's logs, 
proving bucket sharding worked.

**3. NoLostRequests**

**Count invariant:**

Lists all records via `ListRecords(parent: "default/results/-")` with pagination. Counts records matching TaskRuns from the test. Asserts exactly 9 records found.

**Individual retrieval:**

For each TaskRun, calls GetRecord using the record name from the annotation. Asserts every call succeeds, proving records are persisted and retrievable.

**Data integrity:**

For each Record, unmarshals the `data` field to a TaskRun and verifies the `Name` field matches the expected TaskRun name. Catches corruption during concurrent writes.

**4. APIDistribution**

Reads logs from all API pods via Kubernetes Logs API. Counts log lines containing `"grpc.method"` per pod. Asserts at least 2 of 3 API pods received gRPC requests.
  
Prints per-pod request counts for debugging:

```
API pod tekton-results-api-abc123: 15 requests
API pod tekton-results-api-def456: 12 requests
API pod tekton-results-api-ghi789: 14 requests
```

