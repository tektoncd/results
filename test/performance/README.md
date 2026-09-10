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
baselines/     committed baseline reports (follow-up)
```

`generator`, `metrics`, and `report` are importable packages — the compare tool
(Story 02) and compliance suite (Story 03) reuse the generator, datasets, and
report schema.

## Quick start (tier-1 kind, local DB)

```bash
# 1. Cluster + install with fixed Postgres tuning
./test/performance/environments/kind/00-kind-up.sh
./test/performance/environments/kind/01-install-localdb.sh

# The install prints the cert/token paths; export them for convenience:
export SSL_CERT_PATH=/tmp/tekton-results/ssl
export SA_TOKEN_PATH=/tmp/tekton-results/tokens
export API_SERVER_ADDR=https://localhost:8080

CERT="${SSL_CERT_PATH}/tekton-results-cert.pem"
TOKEN="${SA_TOKEN_PATH}/all-namespaces-admin-access"

# 2. Load + verify the seed dataset through the API
go run ./test/performance/harness load --verify --cert "$CERT" --token "$TOKEN"

# 3. Benchmark
go run ./test/performance/harness store  --cert "$CERT" --token "$TOKEN" --output store.json
go run ./test/performance/harness query  --cert "$CERT" --token "$TOKEN" --output query.json --transport both
go run ./test/performance/harness mixed  --cert "$CERT" --token "$TOKEN" --output mixed.json --duration 60
```

## Subcommands

| Command   | What it measures                                                        |
| --------- | ---------------------------------------------------------------------- |
| `load`    | Loads the seed dataset via the API; `--verify` checks row counts against the golden definition. |
| `store`   | Write/ingest — replays the watcher lifecycle (CreateResult → CreateRecord(pending) → 1–3 UpdateRecord → UpdateResult → 2–15 child records) over the **live** UID range. |
| `query`   | Read/list — a fixed CEL query mix (by status, recent window, by type, by label) against the **seed** range, paginated. `--transport grpc\|rest\|both`. |
| `mixed`   | Writers (live range) and readers (seed range) in parallel; sized by `--read-ratio`/`--write-ratio`. |
| `dataset` | Emits the golden `DatasetDefinition` JSON — no cluster required. |

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
| `--db-backend`    | `local`                             | `BENCH_DB_BACKEND`|
| `--output`        | stdout                               |                   |

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

## Report schema

Every run emits a JSON `Report` (`report.SchemaVersion`) — the comparison input
for Story 02:

```jsonc
{
  "schema_version": "1",
  "meta": {
    "git_commit": "…", "git_dirty": false,
    "dataset_version": "seed-small-v1", "dataset_hash": "…",
    "tier": "tier1", "mode": "store",
    "started_at": "…", "duration_ms": 1234,
    "hostname": "…", "api_server_addr": "…",
    "db_backend": "local", "server_image": "…"
  },
  "config": { "count": 1000, "concurrency": 8, "transport": "grpc", "seed": 42, … },
  "metrics": {
    "throughput_per_sec": 812.3,
    "total_ops": 5000, "total_errors": 0,
    "by_op": {
      "create_record": { "count": 1000, "errors": 0, "error_codes": {},
        "p50_ms": 3.1, "p90_ms": 7.4, "p99_ms": 19.0,
        "min_ms": 1.2, "max_ms": 41.0, "mean_ms": 4.0 }
    }
  }
}
```

Percentiles are exact (nearest-rank over sorted samples); errors are classified by
gRPC status code. Map keys serialize in stable order.

## External database

To benchmark against a managed/external Postgres instead of the in-cluster one:

```bash
export BENCH_DB_URL="postgres://user:pass@host:5432/results?sslmode=require"
./test/performance/environments/kind/02-install-externaldb.sh
```

This rewires only configuration (`DB_*` config + credentials secret) — no code or
image change — and sets `db_backend=external` in reports. The harness is unchanged
because it only talks to the API.

## Comparing runs

Reports are self-describing (git commit, dataset hash, tier, DB backend). To
compare two runs, diff `metrics.by_op[*].p99_ms` and `throughput_per_sec` for the
same `mode` and `dataset_hash`. Guideline variance for a pass/fail gate is ±10% on
p99. Committed reference runs live in [`baselines/`](baselines/) (populated as a
follow-up).

## Tests

```bash
go test ./test/performance/...
```

Covers generator determinism and distributions, UID non-overlap, metrics
percentile math, report round-trip, and harness workload wiring. The CI smoke run
stands up local-db kind, loads `seed-small`, then runs `store|query|mixed` capped
at 1k records and asserts each emits valid JSON.
