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
- helm (>= 3.12)

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

### `02-loki-vector.sh`

Installs single-binary Loki and Vector (Helm) in namespace `logging`, applies
`loki_vector/loki-vector-api-config.yaml` (`LOGS_TYPE=Loki`), and restarts the
Results API so the v1alpha3 log plugin queries Loki.

Used by `e2e.sh` after the default e2e and GCS suites. Requires Helm.

### `03-splunk.sh`

Installs Splunk (Free license) and Vector (Helm) in namespace `logging`, sets
`SPLUNK_SEARCH_TOKEN`, applies `splunk/api-config.yaml` (`LOGS_TYPE=Splunk`),
and restarts the Results API so the v1alpha3 log plugin queries Splunk.

Requires Helm and Docker (the Splunk image is loaded into kind). Do not run on
the same cluster as Loki: both overwrite `tekton-results-api-config` and the
`vector` Helm release.

### `e2e-splunk.sh`

Creates a kind cluster, runs `00-setup.sh`, `01-install.sh`, and `03-splunk.sh`,
then `go test --tags=e2e,splunk`. Deletes the cluster on exit.

Used by the Nightly Splunk E2E GitHub Action
(`.github/workflows/nightly-splunk-e2e.yaml`). The `schedule` trigger only runs
after this workflow is on the default branch; use **Run workflow**
(`workflow_dispatch`) to try it before that. Splunk is not part of presubmit.

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


## Log plugin backend matrix

| Backend | Setup | Go tags | CI |
| --- | --- | --- | --- |
| Default Results APIs (no plugin logs) | `00-setup.sh` + `01-install.sh` | `e2e` | presubmit (`e2e.sh`) |
| GCS emulator (legacy v1alpha2 `GetLog`, deprecated) | `gcs-emulator.yaml` | `e2e,gcs` | presubmit (`e2e.sh`) |
| Loki + Vector (plugin v1alpha3) | `02-loki-vector.sh` | `e2e,loki` | presubmit (`e2e.sh`, same Integration Tests job) |
| Splunk + Vector (plugin v1alpha3) | `03-splunk.sh` (`e2e-splunk.sh`) | `e2e,splunk` | nightly (`.github/workflows/nightly-splunk-e2e.yaml`; not every PR) |

`--tags=e2e` does **not** run Loki or Splunk plugin tests. Those files are `//go:build e2e && loki` and `//go:build e2e && splunk`.

Shared helpers: `logs_harness_test.go`. Fixture: `testdata/pipelinerun-plugin-logs.yaml`.

### Run Loki locally (kind)

From the **results** repo root (`git rev-parse --show-toplevel` must be this repo):

```sh
export KIND_CLUSTER_NAME=tekton-results
export KO_DOCKER_REPO=kind.local
export SA_TOKEN_PATH=/tmp/tekton-results/tokens
export SSL_CERT_PATH=/tmp/tekton-results/ssl
export SSL_INCLUDE_LOCALHOST=true
export API_SERVER_ADDR=https://localhost:8080
export CGO_ENABLED=0

./test/e2e/00-setup.sh
./test/e2e/01-install.sh
./test/e2e/02-loki-vector.sh

cd test/e2e
go test -v -count=1 --tags=e2e,loki . -timeout 15m
```

### Run Splunk locally (kind)

Same env as Loki, then:

```sh
./test/e2e/00-setup.sh
./test/e2e/01-install.sh
./test/e2e/03-splunk.sh

cd test/e2e
go test -v -count=1 --tags=e2e,splunk . -timeout 20m
```

Or from the repo root: `./test/e2e/e2e-splunk.sh` (deletes the kind cluster on exit).