<!--
---
linkTitle: "Performance Benchmarking"
weight: 305
---
-->

# Performance Benchmarking

Tekton Results ships a repeatable harness that measures **store** (write and
ingest) and **query** (read and list) performance of a deployed instance. You
run it to prove an improvement or to catch a regression across schema changes
(metadata columns, label tables, indexes) and new code paths.

The harness drives only the deployed API — gRPC for writes, REST or gRPC for
reads. It never touches the database directly, so every measurement is an
end-to-end, client-observed latency for one API operation. Because the API
server is a thin layer over Postgres, these numbers act as a proxy for database
performance: with the dataset held constant, a change between two runs is the
change in the underlying store.

The framework lives in [`test/performance/`](../test/performance/). This page
introduces it; the [framework README](../test/performance/README.md) documents
every flag, and the [seed dataset spec][seed-small] records the golden
fingerprint.

[seed-small]: ../test/performance/datasets/seed-small.md

## Prerequisites

- A running Tekton Results deployment. The harness reaches it over the
  address in `<API_SERVER_ADDR>` (default `https://localhost:8080`).
- A client certificate and a service account token with access to the
  Results API. The install scripts under
  [`environments/kind/`](../test/performance/environments/kind/) print their
  paths.
- Go, to run the harness with `go run`.

You do not need database credentials. The harness only speaks to the API.

## Run a benchmark

Stand up a local `kind` cluster and install Tekton Results with fixed Postgres
tuning:

```bash
./test/performance/environments/kind/00-kind-up.sh
./test/performance/environments/kind/01-install-localdb.sh
```

Export the certificate and token paths the install prints:

```bash
export API_SERVER_ADDR=https://localhost:8080
CERT=/tmp/tekton-results/ssl/tekton-results-cert.pem
TOKEN=/tmp/tekton-results/tokens/all-namespaces-admin-access
```

Load the versioned seed dataset through the API and verify its row counts
against the golden definition:

```bash
go run ./test/performance/harness load --verify --cert "$CERT" --token "$TOKEN"
```

Run the store, query, and mixed workloads. Each command writes a JSON report
under `test/performance/output/`, which the harness creates on demand. Omit
`--output` to stream the report to stdout instead:

```bash
go run ./test/performance/harness store --cert "$CERT" --token "$TOKEN" \
  --output test/performance/output/store.json
go run ./test/performance/harness query --cert "$CERT" --token "$TOKEN" \
  --output test/performance/output/query.json --transport both
go run ./test/performance/harness mixed --cert "$CERT" --token "$TOKEN" \
  --output test/performance/output/mixed.json --duration 60
```

The `load --verify` command recomputes the dataset content hash and reports
whether the loaded record counts match the golden definition.

## Subcommands

Each subcommand drives one workload:

- `load` loads the seed dataset via the API. With `--verify` it checks row
  counts against the golden definition.
- `store` measures writes by replaying the watcher lifecycle over the live UID
  range.
- `query` measures reads with a fixed CEL query mix against the seed range.
  Select the protocol with `--transport grpc`, `--transport rest`, or
  `--transport both`.
- `mixed` runs writers and readers in parallel, sized by `--read-ratio` and
  `--write-ratio`.
- `dataset` prints the golden dataset definition as JSON. It needs no cluster.

## Read a report

Every run emits a JSON report. The `metrics` block carries a
`throughput_per_sec` headline rate and a `by_op` breakdown, because a single
write instantiates several distinct calls with very different costs. Each
operation reports latency percentiles over its sorted samples:

- `p50_ms` is the median: half of calls were this fast or faster.
- `p90_ms`: only the slowest 10% of calls were worse.
- `p99_ms` is the slow tail. A regression such as a missing index often moves
  `p99_ms` long before it touches `p50_ms`, so the pass/fail gate uses `p99_ms`,
  not the mean.

Percentiles stabilize with sample count. Trust `p99_ms` at the thousands of
samples the seed dataset produces, not at a few dozen. The harness is
black-box: it tells you *that* something regressed, not *which* query. To
attribute a regression to a specific statement, use server-side tooling such as
`pg_stat_statements` or `EXPLAIN ANALYZE`.

## Compare runs

Reports are self-describing: each records the git commit, dataset hash, tier,
and database backend. To compare two runs, diff `throughput_per_sec` and
`metrics.by_op[*].p99_ms` for the same `mode` and the same `dataset_hash`. The
guideline variance for a pass/fail gate is ±10% on `p99_ms`. Runs are comparable
only within the same dataset version.

## Benchmark against an external database

To benchmark against a managed or external Postgres instead of the in-cluster
one, point `<BENCH_DB_URL>` at it and run the external-database installer:

```bash
export BENCH_DB_URL="postgres://<user>:<pass>@<host>:5432/results?sslmode=require"
./test/performance/environments/kind/02-install-externaldb.sh
# The installer cannot export into this shell; set the report label here:
export BENCH_DB_BACKEND=external
```

This rewires configuration only — the `DB_*` settings and the credentials
secret — with no code or image change. Exporting `BENCH_DB_BACKEND=external`
(or passing `--db-backend=external`) labels reports with `db_backend=external`.
The harness itself is unchanged because it only talks to the API.
