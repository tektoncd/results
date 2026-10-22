# Tekton Results — Performance Benchmark Framework

A repeatable harness for measuring **store** (write/ingest) and **query**
(read/list) performance of Tekton Results — separately and in parallel — always
starting from an identical, versioned seed dataset. It drives the **real deployed
API** only (gRPC for writes, REST/gRPC for reads); it never touches the database
directly.

Use it to prove improvement and catch regressions across schema changes (metadata
columns, label tables, indexes) and new code paths.

```
generator/     deterministic PipelineRun/TaskRun generator + golden DatasetDefinition
metrics/       latency percentiles, error-by-code counters, throughput (importable)
report/        JSON report schema + metadata capture (importable)
harness/       cobra "bench" CLI + store/query/mixed/load drivers (package main)
datasets/      dataset specs (see seed-small.md)
environments/  kind cluster + install automation (local & external DB)
kustomize/     benchmark deployment overlays with API client rate limits
```

## Prerequisites

- A running Tekton Results deployment. The harness reaches it over the
  address in `<API_SERVER_ADDR>` (default `https://localhost:8080`).
- A client certificate and a service account token with access to the
  Results API. The install scripts under
  [`environments/kind/`](../test/performance/environments/kind/) print their
  paths.
- Go >= 1.26.

You do not need database credentials. The harness only speaks to the API.


## Quick start (tier-1 kind, local DB)

```bash
# 1. Cluster + install with fixed Postgres tuning
./test/performance/environments/kind/00-kind-up.sh
./test/performance/environments/kind/01-install-localdb.sh
```

Both benchmark installers share the settings in
[`api-client-tuning/kustomization.yaml`](kustomize/api-client-tuning/kustomization.yaml).
Edit `K8S_QPS`, `K8S_BURST`, and `AUTH_DISABLE` directly in that file, then rerun
the appropriate installer to apply the changes. These values are API container
environment variables that override its config-file defaults.

The default values are `K8S_QPS=50`, `K8S_BURST=100`, and `AUTH_DISABLE=false`.
To bypass the API's Kubernetes authentication and RBAC checks for a benchmark,
change the `AUTH_DISABLE` entry to `value: "true"`. TLS remains enabled; use the
usual harness certificate and token arguments.

The local-DB overlay extends the e2e deployment; the
[`external-db` overlay](kustomize/external-db/) deploys without in-cluster Postgres.
To inspect the manifests before installing:

```bash
kubectl kustomize test/performance/kustomize
kubectl kustomize test/performance/kustomize/external-db
```

Check if the api server address, certificate and token paths 
environment variables (should have been exposed by the 
previously executed 01-install-localdb.sh script):

```bash
export SSL_CERT_PATH=/tmp/tekton-results/ssl
export SA_TOKEN_PATH=/tmp/tekton-results/tokens
export API_SERVER_ADDR=https://localhost:8080
```
Load the versioned seed dataset through the API and verify its row counts
against the golden definition:

```bash
# 2. Load + verify the seed dataset through the API
go run ./test/performance/harness load --verify --cert "$SSL_CERT_PATH" --token "$SA_TOKEN_PATH"

# 3. Benchmark — reports are written under test/performance/output/ (created on
#    demand); omit --output to write under /tmp/<timestamp>-<commit> instead.
go run ./test/performance/harness store  --cert "$SSL_CERT_PATH" --token "$SA_TOKEN_PATH" --output test/performance/output/store.json
go run ./test/performance/harness query  --cert "$SSL_CERT_PATH" --token "$SA_TOKEN_PATH" --output test/performance/output/query.json --transport both
go run ./test/performance/harness mixed  --cert "$SSL_CERT_PATH" --token "$SA_TOKEN_PATH" --output test/performance/output/mixed.json --duration 60
```

## Subcommands

| Command   | What it measures                                                        |
| --------- | ---------------------------------------------------------------------- |
| `load`    | Loads the seed dataset via the API; `--verify` checks row counts against the golden definition. |
| `store`   | Write/ingest — replays the watcher lifecycle (CreateResult → CreateRecord(pending) → 1–3 UpdateRecord → UpdateResult → 2–15 child records) over the **live** UID range. |
| `query`   | Read/list — a fixed CEL query mix (by status, recent window, by type, by label) against the **seed** range, paginated. `--transport grpc\|rest\|both`. |
| `mixed`   | Writers (live range) and readers (seed range) in parallel; sized by `--read-ratio`/`--write-ratio`. |
| `dataset` | Emits the golden `DatasetDefinition` JSON — no cluster required. |

When `load` or `store` uses `--output report.json`, it also writes
`report.sendlog.json` with the instance UIDs, record names, and operation status
codes for ingest verification. The load report and send log are written before
`--verify` runs, so they remain available if verification fails. No sidecar is
written when `--output` is omitted.

Every `load` report includes `metrics.seed_load` with `total_instances`,
`failed_instances`, and `error_rate_percent`.
An instance comprises one PipelineRun and its child TaskRuns; any error while
writing its Result, PipelineRun, or children counts that instance once.
`AlreadyExists` responses tolerated by the loader do not count as failed
instances. The summary describes write outcomes, not an actual missing-row count.
Write errors do not fail the command; `--verify` still fails if the stored row
counts do not match the golden definition. See [Investigating seed-load errors](#investigating-seed-load-errors)
for how to find the underlying error in API and Postgres logs.

## Common flags

Defaults come from the same environment variables the e2e suite uses.

| Flag              | Default                              | Env               |
| ----------------- | ------------------------------------ | ----------------- |
| `--server-addr`   | `https://localhost:8080`             | `API_SERVER_ADDR` |
| `--server-name`   | `tekton-results-api-service…svc…`    | `API_SERVER_NAME` |
| `--cert`          | —                                    | `SSL_CERT_PATH`   |
| `--token`         | —                                    | `SA_TOKEN_PATH`   |
| `--concurrency`   | `8`                                  |                   |
| `--count`         | `1000` (count-bounded runs)          |                   |
| `--duration`      | `0` → 30s default for time-bounded   |                   |
| `--seed`          | `42`                                 |                   |
| `--namespaces`    | `50`                                 |                   |
| `--child-min/max` | `2` / `15`                           |                   |
| `--db-type`    | `local`                              | `BENCH_DB_TYPE`   |
| `--output`        | `/tmp/<timestamp>-<commit>`           |                   |

## Dataset determinism

The generator is seeded (`math/rand/v2` PCG per index) and uses a fixed absolute
time window — never `time.Now()` — so a given `(seed, count, namespaces)` always
produces byte-identical objects and a stable `content_hash`. Seed data and live
(store/mixed) data draw UIDs from **disjoint** ranges so writes never collide with
loaded reads. See [`datasets/seed-small.md`](datasets/seed-small.md) for the
tier-1 spec and golden fingerprint.

Templates are anonymized real PipelineRun/TaskRun manifests dropped into
[`generator/templates/`](generator/templates/) per the contract documented there;
a sample ships so the framework builds and tests run before real manifests arrive.
Each template may carry a weighted set of child TaskRun skeletons (a `taskruns/`
directory plus `childSkeletonWeights`), so generated PipelineRuns reproduce a
realistic child-record size spread rather than repeating one identical TaskRun.

## Report schema

Every run emits a JSON `Report` (`report.SchemaVersion`) — the comparison input
for later runs:

```jsonc
{
  "schema_version": "1",
  "meta": {
    "git_commit": "…", "git_dirty": false,
    "dataset_version": "seed-small-v1", "dataset_hash": "…",
    "tier": "tier1", "mode": "load",
    "started_at": "…", "duration_ms": 1234,
    "api_server_addr": "…",
    "db_type": "local", "server_image": "…"
  },
  "config": { "count": 1000, "concurrency": 8, "transport": "grpc", "seed": 42, … },
  "metrics": {
    "throughput_per_sec": 812.3,
    "total_ops": 5000, "total_errors": 0,
    "seed_load": {
      "total_instances": 1000, "failed_instances": 0, "error_rate_percent": 0
    },
    "by_op": {
      "create_record": { "count": 1000, "errors": 0, "error_codes": {},
        "p50_ms": 3.1, "p90_ms": 7.4, "p99_ms": 19.0,
        "min_ms": 1.2, "max_ms": 41.0, "mean_ms": 4.0 }
    }
  }
}
```

Percentiles are exact (nearest-rank over sorted samples); errors are classified by
gRPC status code. Map keys serialize in stable order. `metrics.seed_load` is
present only in `load` reports. Its percentage is
`100 * failed_instances / total_instances`, or zero when no instances were
attempted. Per-operation error counts and codes remain in `metrics.by_op`.
Those counters record raw API statuses, so tolerated `AlreadyExists` responses
can appear there even when `failed_instances` is zero.

### Investigating seed-load errors

Start with the instance summary and the operations with nonzero error counts:

```bash
# Replace this with the path of your load report.
report_path=/tmp/20261022-120000-abcdef01
jq '{seed_load: .metrics.seed_load,
     failed_operations: (.metrics.by_op | with_entries(select(.value.errors > 0)))}' "$report_path"
```

The report contains aggregate counts and gRPC status codes. For the underlying
server or database error, collect logs covering the run's start time. These
selectors and container names match the bundled performance deployment:

```bash
run_start=$(jq -r '.meta.started_at' "$report_path")

kubectl logs -n tekton-pipelines \
  -l app.kubernetes.io/name=tekton-results-api -c api \
  --since-time="$run_start" --timestamps --prefix --tail=-1 > api.log

kubectl logs -n tekton-pipelines \
  -l app.kubernetes.io/name=tekton-results-postgres -c postgres \
  --since-time="$run_start" --timestamps --prefix --tail=-1 > postgres.log
```

Inspect `api.log` for request failures, timeouts, and database errors. Inspect
`postgres.log` for `ERROR`, `FATAL`, deadlocks, connection-limit errors, or failed
statements around the same time. Use the report's `meta.duration_ms` to identify
the run's time window, and the instance UIDs/resource names in the send log, when
available, to correlate entries. Authentication, validation, or transport errors
can occur before a database request, so they may have no matching Postgres entry.

If a container restarted, request its previous logs with `kubectl logs --previous`
using that pod and container name. For the external database overlay, use the
database's actual namespace, pod selector, and container name; for a managed
database, inspect its provider's database logs.

### What the measurements mean

The harness only ever calls the deployed API, so operation metrics are **end-to-end,
client-observed** measurements of API operations (`create_result`,
`create_record`, `update_record`, `create_child`, the list queries, …). It is a
**proxy for database performance**: the API server is a thin layer over Postgres,
so with the dataset held constant (same `dataset_hash`) a change in these numbers
between two runs is the change in the underlying store.

- **`throughput_per_sec`** — total API operations completed divided by the run's
  wall-clock window (`meta.duration_ms`). The headline rate; higher is better.
- **`total_ops` / `total_errors`** — operation and failure counts across every op.
- **`by_op`** — the same breakdown per operation, because a write instantiates
  several distinct calls with very different cost. `count` is the number of samples
  the percentiles are computed from; `errors`/`error_codes` classify failures by
  gRPC status code.

**Latency percentiles** describe the *distribution* of per-call durations, not an
average — latency is skewed, and a few slow calls that a mean would hide are
exactly what a storage regression looks like. Sort all samples for an operation
fastest→slowest:

- **`p50_ms`** (median) — half of calls were this fast or faster; the typical case.
- **`p90_ms`** — 90% were this fast or faster; only the slowest 10% were worse.
- **`p99_ms`** — 99% were this fast or faster; the slow **tail**. Regressions
  (e.g. a missing index) often move p99 long before they touch p50, which is why
  the pass/fail gate below is on p99, not the mean.
- **`min_ms` / `max_ms` / `mean_ms`** — best case, worst single sample, and
  arithmetic average. `mean` is included for reference only; prefer the
  percentiles when comparing runs.

Percentiles stabilize with sample count — trust `p99` at the thousands-of-samples
scale the seed dataset produces, not at a few dozen (where `p99` is just the single
slowest call). To attribute a regression to a specific SQL statement or query plan
you still need server-side DB tooling (`pg_stat_statements`, `EXPLAIN ANALYZE`);
this harness is black-box and tells you *that* something regressed, not *which*
query.

## External database

To benchmark against a managed/external Postgres instead of the in-cluster one:

```bash
export BENCH_DB_URL="postgres://user:pass@host:5432/results?sslmode=require"
./test/performance/environments/kind/02-install-externaldb.sh
# The installer cannot export into this shell; set the report label here:
export BENCH_DB_TYPE=external
```

This rewires only configuration (`DB_*` config + credentials secret) — no code or
image change. Exporting `BENCH_DB_TYPE=external` (or passing
`--db-type=external`) labels reports with `db_type=external`. The harness
is unchanged because it only talks to the API.

## Comparing runs

Reports are self-describing (git commit, dataset hash, tier, DB backend). To
compare two runs, diff `metrics.by_op[*].p99_ms` and `throughput_per_sec` for the
same `mode` and `dataset_hash`. Guideline variance for a pass/fail gate is ±10% on
p99.

## Tests

```bash
go test ./test/performance/...
```

Covers generator determinism and distributions, UID non-overlap, metrics
percentile math, report round-trip, and harness workload wiring. 

## Smoke tests

The CI smoke run stands up local-db kind, loads `seed-small`, then runs `store|query|mixed` capped
at 1k records and asserts each emits valid JSON.

```bash
./test/performance/environments/kind/03-smoke.sh
```

Currently it can be executed manually, but later will be integrated to CI workflows (TBD).
