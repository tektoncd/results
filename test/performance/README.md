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
environments/  kind (tier-1) + openshift (tier-2) cluster & install automation
harness/workload/  anonymized query-mix spec + KubeArchive-log derivation
results/       committed milestone-gate results + process (see results/README.md)
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

# 3. Benchmark — reports are written under test/performance/output/ (created on
#    demand); omit --output to stream the JSON report to stdout instead.
go run ./test/performance/harness store  --cert "$CERT" --token "$TOKEN" --output test/performance/output/store.json
go run ./test/performance/harness query  --cert "$CERT" --token "$TOKEN" --output test/performance/output/query.json --transport both
go run ./test/performance/harness mixed  --cert "$CERT" --token "$TOKEN" --output test/performance/output/mixed.json --duration 60
```

## Subcommands

| Command   | What it measures                                                        |
| --------- | ---------------------------------------------------------------------- |
| `load`    | Loads the seed dataset via the API; `--verify` checks row counts against the golden definition. |
| `store`   | Write/ingest — replays the watcher lifecycle (CreateResult → CreateRecord(pending) → 1–3 UpdateRecord → UpdateResult → 2–15 child records) over the **live** UID range. |
| `query`   | Read/list against the **seed** range, paginated. Replays the committed, KubeArchive-derived query mix by default; `--query-spec` overrides, `--legacy-queries` uses the old fixed mix. `--transport grpc\|rest\|both`. |
| `mixed`   | Writers (live range) and readers (seed range) in parallel; sized by `--read-ratio`/`--write-ratio`. |
| `dataset` | Emits the golden `DatasetDefinition` JSON — no cluster required. |
| `derive`  | Turns raw KubeArchive access logs into an anonymized query-mix spec — no cluster required. |
| `compare` | Diffs two JSON reports and flags regressions; exits non-zero when the gate fails — no cluster required. |

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
Each template may carry a weighted set of child TaskRun skeletons (a `taskruns/`
directory plus `childSkeletonWeights`), so generated PipelineRuns reproduce a
realistic child-record size spread rather than repeating one identical TaskRun.

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

### What the measurements mean

The harness only ever calls the deployed API, so every number is an **end-to-end,
client-observed** measurement of one API operation (`create_result`,
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
export BENCH_DB_BACKEND=external
```

This rewires only configuration (`DB_*` config + credentials secret) — no code or
image change. Exporting `BENCH_DB_BACKEND=external` (or passing
`--db-backend=external`) labels reports with `db_backend=external`. The harness
is unchanged because it only talks to the API.

## Comparing runs

Reports are self-describing (git commit, dataset hash, tier, DB backend). Use the
`compare` subcommand to diff a baseline against a candidate:

```bash
go run ./test/performance/harness compare baseline.json candidate.json
# or loosen/tighten the gate:
go run ./test/performance/harness compare --threshold 0.15 baseline.json candidate.json
```

It compares `metrics.by_op[*].p99_ms` (lower is better) and `throughput_per_sec`
(higher is better), warns when `mode` or `dataset_hash` differ (numbers are then
not strictly comparable), and **exits non-zero** when any metric regresses beyond
the threshold — so CI and the milestone gates can consume it directly. Guideline
variance is ±10% on p99 (the default); the tier-2 threshold is TBD until the first
ephemeral runs establish run-to-run variance. Committed reference runs live in
[`baselines/`](baselines/) and promoted milestone results in
[`results/`](results/README.md) (both populated as a follow-up).

## Query workload (derived from KubeArchive logs)

The `query` driver replays a **query mix derived from real KubeArchive deployment
access logs** rather than a hand-guessed set. The committed spec lives at
[`harness/workload/querymix.yaml`](harness/workload/querymix.yaml) and is embedded
in the binary, so `query` is spec-driven with no extra flags.

The spec is **anonymized**: it records only operation types, resource kinds,
label-selector *keys*, page-size distribution, and namespace fan-out as counts and
weights. Raw namespaces, label values, tokens, hostnames, and UIDs are never
stored, so no identifiers from the source deployment leak into the repo. Regenerate
it from logs with `derive` (raw logs are never committed):

```bash
go run ./test/performance/harness derive --input access.log --output harness/workload/querymix.yaml
```

### Capturing a real access log

`derive` reads the KubeArchive API server's **structured (slog) access log** — the
`msg="Served request"` lines emitted by `pkg/middleware/logging.go`, where the verb
and URL-encoded path are discrete `method=`/`path=` fields. Non-access-log noise
(klog throttling, http2 preface errors, trace-export failures), write verbs, and
health/metrics paths are ignored automatically, so the raw log needs no
pre-cleaning. Capture a representative window from a cluster with real traffic:

```bash
# From pod logs (needs enough retention to cover a busy window):
kubectl -n kubearchive logs deploy/kubearchive-api-server --since=3h > access.log
# On production, prefer the central logging backend (Loki/Splunk/Vector) for the
# kubearchive-api-server component so the window isn't truncated by log rotation.
```

Treat `access.log` as **sensitive** (it contains real namespaces, label values, and
UIDs) and keep it **outside the repo** — only the derived, anonymized spec is
committed. After deriving, verify nothing leaked before committing `querymix.yaml`:

```bash
go run ./test/performance/harness derive --input access.log --output harness/workload/querymix.yaml
grep -Ei '<a-real-namespace>|<a-real-app>|[0-9a-f]{8}-[0-9a-f]{4}' harness/workload/querymix.yaml  # expect no matches
```

An anonymized sample log ships at
[`harness/workload/testdata/sample-access.log`](harness/workload/testdata/sample-access.log)
and reproduces the committed spec. Override the spec per-run with
`--query-spec <file>`, or fall back to the historic hardcoded mix with
`--legacy-queries`. Native Kubernetes-style API replay (namespaces/kind paths) is
out of scope until that API lands (epic Stories 14–16); today each spec operation
is mapped to the closest Results/Records query.

## Tier-2 (ephemeral OpenShift)

Tier 2 is a multi-node ephemeral OpenShift cluster with a 5–10 GB dataset, run
inside a ~90-minute session budget — large enough to expose planner regressions,
index bloat, and lock contention that tier-1 kind (<1 GB) cannot. The phase scripts
live in [`environments/openshift/`](environments/openshift/README.md); each accepts
`--cluster kind|openshift` (default `openshift`) so the whole pipeline can be
dry-run on kind before booking cluster time.

```bash
cd test/performance/environments/openshift
export BENCH_CLUSTER=openshift          # or: kind, for a local dry run
./00-cluster.sh                         # validate the booked cluster
./01-install.sh                         # install + tier-2 Postgres tuning
./02-load.sh                            # parallel seed load (5-10 GB)
./03-benchmark.sh                       # store/query/mixed at --tier tier2
```

Capturing the tier-2 5–10 GB baseline and committing milestone result JSONs under
[`results/`](results/README.md) is a follow-up that needs a live cluster.

## Tests

```bash
go test ./test/performance/...
```

Covers generator determinism and distributions, UID non-overlap, metrics
percentile math, report round-trip, and harness workload wiring. 

## Smoke tests

The smoke run stands up local-db kind, loads `seed-small`, then runs
`store|query|mixed` capped at 1k records and asserts each emits valid JSON. It is a
functional check, **not** a performance gate.

```bash
make performance-smoke
# or directly:
./test/performance/environments/kind/03-smoke.sh
```

CI runs it on demand via the **Performance Smoke** GitHub Actions workflow
(`workflow_dispatch`, see
[`.github/workflows/performance-smoke.yaml`](../../.github/workflows/performance-smoke.yaml)),
which stands up kind local-db and runs `make performance-smoke`. Tier-2 runs stay
manual (they need a booked ephemeral OpenShift cluster).
