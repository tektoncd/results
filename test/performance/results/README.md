# Committed benchmark results & milestone gates

This tree holds the **promoted** benchmark reports that back the epic's
milestone-comparison process (SRVKP-13131). A run is promoted here only after it
has been validated on a real environment; the raw `output/` directory is
throwaway.

## Layout

```
results/<milestone>/<tier>-<dataset-version>-<commit>.json
```

- `<milestone>` — the epic phase the run gates (see the table below), e.g. `indexing-07`.
- `<tier>` — `tier1` (kind, <1 GB) or `tier2` (ephemeral OpenShift, 5–10 GB).
- `<dataset-version>` — the dataset content tag (e.g. `seed-large-v1`); a change to
  the generator, dataset, or `postgresql.conf` is a dataset-version bump.
- `<commit>` — the short git SHA the server was built from.

Each file is a `report.Report` JSON as emitted by `bench` (`--output`). Compare two
of them with:

```bash
go run ./test/performance/harness compare \
  results/indexing-07/tier2-seed-large-v1-<baseline>.json \
  output/tier2/query.json
```

`compare` exits non-zero when any `p99_ms` or `throughput_per_sec` regresses beyond
the threshold (default ±10%; tier-2 threshold TBD after the first ephemeral runs
establish run-to-run variance).

## Milestone gates

Each epic phase captures a tier-2 baseline before its change and a candidate after,
then gates on `compare`. Expected outcome is the direction the phase is meant to
move the numbers.

| Milestone group        | Phases  | Change under test                          | Expected outcome                                  |
| ---------------------- | ------- | ------------------------------------------ | ------------------------------------------------- |
| Indexing               | 07–10   | New/adjusted DB indexes on hot columns     | Query p99 **down**; store p99 no worse than +10%  |
| Label Selector         | 11–13   | Label storage + selector query path        | Label-list p99 **down**; no store regression      |
| K8s-like API           | 14–16   | Kubernetes-style list/get API surface      | New API within target p99; existing modes stable  |
| Generic Storage        | —       | Pluggable storage backend                  | No regression vs the Postgres baseline            |

Per-milestone subdirectories and their result JSONs are added as each phase runs on
a live cluster (a follow-up; this story ships the process and tooling). Until then
the tree is intentionally empty except for this README.
