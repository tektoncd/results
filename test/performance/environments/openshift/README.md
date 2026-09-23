# Tier-2 environment (ephemeral OpenShift)

Automation for the **tier-2** performance environment: a multi-node ephemeral
OpenShift cluster with a 5–10 GB dataset, run within a ~90-minute session budget.
Tier 2 exposes planner regressions, index bloat, and lock contention that the
tier-1 kind environment (<1 GB) is too small to surface.

Every phase accepts `--cluster kind|openshift` (default `openshift`) so the whole
pipeline can be **dry-run locally on kind** before booking OpenShift time. In kind
mode the phases fall back to smaller counts and skip OpenShift-only assumptions
(multi-node, dedicated database node); kind results are dev-only, never a tier-2
baseline.

## Phases

| Script            | Phase | Does                                                                 |
| ----------------- | ----- | ------------------------------------------------------------------- |
| `00-cluster.sh`   | 0     | Select + validate the target cluster (`--cluster kind\|openshift`). |
| `01-install.sh`   | 1     | Install Results + Postgres (dedicated node + tuning, or `BENCH_DB_URL`). |
| `02-load.sh`      | 2     | Parallel seed load via the API (or `BENCH_SEED_DUMP` restore, query-only). |
| `03-benchmark.sh` | 3     | Run store/query/mixed at `--tier tier2`; validate and write JSON reports. |

`common.sh` is sourced by every phase for `--cluster` resolution and phase-budget
timing; it is not executed directly.

## Safety: production cluster protection

Benchmark operations are **destructive** (database resets, Postgres tuning, high-concurrency
load) and must only run on ephemeral throwaway clusters. `00-cluster.sh` includes safety
checks that refuse to proceed if the cluster looks like a production/working environment:

**Blocked if detected:**
- ❌ Tekton Results installed in `openshift-pipelines` namespace (production deployment)
- ❌ Namespaces matching `production`, `prod-*`, or `staging` patterns
- ❌ Cluster domain doesn't match ephemeral patterns (`*.openshiftapps.com`, `*ephemeral*`, `*tmp-*`, `*test-*`)

**Override:** If a cluster has production markers but is genuinely disposable, set:
```bash
export BENCH_ALLOW_PRODUCTION=yes
./00-cluster.sh
```

**Safe by design:** Ephemeral OpenShift clusters provisioned for benchmarking automatically
pass all checks (domain = `*.openshiftapps.com`, no production namespaces, Results not
pre-installed).

## Usage

```bash
cd test/performance/environments/openshift

# Local dry run of the whole tier-2 pipeline on kind:
export BENCH_CLUSTER=kind
./00-cluster.sh && ./01-install.sh && ./02-load.sh && ./03-benchmark.sh

# Real tier-2 run against a booked cluster (KUBECONFIG points at it):
export BENCH_CLUSTER=openshift
./00-cluster.sh
./01-install.sh
./02-load.sh
./03-benchmark.sh
```

## Tunables (env)

| Variable               | Purpose                                                        |
| ---------------------- | -------------------------------------------------------------- |
| `BENCH_CLUSTER`        | `kind` or `openshift` (or pass `--cluster`).                   |
| `BENCH_BUDGET_SECONDS` | Session budget for the phase-timer warnings (default 5400).    |
| `BENCH_COUNT`          | Top-level PipelineRuns to load/benchmark.                      |
| `BENCH_CONCURRENCY`    | Worker concurrency for load and benchmark.                     |
| `BENCH_NAMESPACES`     | Namespaces to spread the dataset across.                       |
| `BENCH_DURATION`       | Query/mixed run duration (seconds).                            |
| `BENCH_OUTPUT_DIR`     | Where reports are written (default `output/tier2`).            |
| `BENCH_DB_URL`         | Use an external managed Postgres instead of the in-cluster one.|
| `BENCH_SEED_DUMP`      | Restore a pg_dump snapshot as the **query-only** seed.         |

The `tier2-5-10gb` dataset size is reached by tuning `BENCH_COUNT` /
`BENCH_NAMESPACES`; capture the exact values in the committed baseline metadata.

## Establishing and using baselines

After your **first successful tier-2 run**, save the reports as your baseline so future
runs have a reference to compare against (detecting regressions or confirming improvements).

### Save your first baseline

```bash
cd test/performance

# Get the current commit SHA
COMMIT=$(git rev-parse --short HEAD)

# Create baseline directory
mkdir -p baselines/current

# Save the query report (primary performance gate)
cp output/tier2/query.json \
   baselines/current/tier2-seed-large-v1-${COMMIT}.json

# Optionally save store and mixed for full coverage
cp output/tier2/store.json \
   baselines/current/tier2-seed-large-v1-${COMMIT}-store.json
cp output/tier2/mixed.json \
   baselines/current/tier2-seed-large-v1-${COMMIT}-mixed.json

echo "Baseline saved: tier2-seed-large-v1-${COMMIT}.json"
```

The naming convention is `<tier>-<dataset-version>-<commit>.json`:
- `tier2` — this is a tier-2 ephemeral OpenShift run
- `seed-large-v1` — the dataset version (bump when generator/schema/postgresql.conf changes)
- `${COMMIT}` — short SHA of the Results commit the API server was built from

### Compare future runs against the baseline

After making a change (e.g., adding DB indexes, optimizing queries), re-run the benchmark
and compare:

```bash
# Re-run the benchmark
cd test/performance/environments/openshift
./03-benchmark.sh

# Compare against your baseline
cd ../..
BASELINE_COMMIT=<the-sha-from-your-baseline-filename>

go run ./harness compare \
  baselines/current/tier2-seed-large-v1-${BASELINE_COMMIT}.json \
  output/tier2/query.json
```

The `compare` tool prints deltas for each metric (`p99_ms`, `throughput_per_sec`) and
**exits non-zero** if any metric regressed beyond the threshold (default ±10%). This
exit code makes it usable in CI gates and the milestone comparison process.

Example output:
```
Comparing baseline vs candidate (mode=query, threshold=10.0%)

Metric                        Baseline    Candidate   Delta      Status
list_records.p99_ms          234.5       198.2       -15.5%     IMPROVED ✓
get_result.p99_ms            45.2        47.1        +4.2%      OK
throughput_per_sec           1250        1180        -5.6%      OK

No regressions detected.
```

### Promoting baselines to the milestone tree

Once a baseline is validated and becomes the reference for a milestone gate (e.g.,
before/after an indexing change), promote it to the committed milestone tree. See
[`../../results/README.md`](../../results/README.md) for the milestone gate process and
layout (`results/<milestone>/<tier>-<dataset-version>-<commit>.json`).
