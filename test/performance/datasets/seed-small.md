# Dataset: `seed-small` (tier-1)

The tier-1 seed dataset used for CI smoke runs and the committed tier-1 baseline.
It is **deterministic**: the same generator seed always produces byte-identical
objects, so the content hash below is a versioned fingerprint. If any value in
this file changes, treat it as a dataset-version bump and refresh the committed
baseline.

## Parameters

| Parameter        | Value  | Flag           |
| ---------------- | ------ | -------------- |
| PipelineRuns     | 1000   | `--count 1000` |
| RNG seed         | 42     | `--seed 42`    |
| Namespaces       | 50     | `--namespaces 50` |
| Child TaskRuns   | 2–15   | `--child-min 2 --child-max 15` |
| Dataset version  | `seed-small` | `--dataset-version seed-small` |

Reproduce the golden definition with no cluster:

```bash
go run ./test/performance/harness dataset \
  --count 1000 --seed 42 --namespaces 50 > /tmp/seed-small.json
```

## Golden fingerprint

| Field                | Expected value                                                     |
| -------------------- | ----------------------------------------------------------------- |
| `content_hash`       | `4ab4cbcfbdbfe4e89e455eaf13971edef720d6fa1e24a8fbedf7e53d3dc60fa5` |
| PipelineRun records  | 1000                                                              |
| TaskRun records      | 3028                                                             |
| Total records        | 4028                                                             |

The content hash covers only the generated objects (UIDs, namespaces, labels,
outcomes, timestamps), never wall-clock metadata, so it is stable across runs and
machines. `bench load --verify` recomputes it and asserts the loaded record
counts match this definition.

## Distributions

Outcomes (target 85% / 10% / 5%):

| Outcome    | Count | Share  |
| ---------- | ----- | ------ |
| succeeded  | 845   | 84.5%  |
| failed     | 101   | 10.1%  |
| cancelled  | 54    | 5.4%   |

Namespaces: 1000 PipelineRuns spread across `ns-00`..`ns-49` (~20 each).

Labels — a small set of hot values plus a long tail:

`appstudio.openshift.io/component`

| Value    | Count |
| -------- | ----- |
| backend  | 413   |
| frontend | 401   |
| db       | 33    |
| api      | 24    |
| auth     | 23    |
| cache    | 23    |
| ingest   | 22    |
| worker   | 22    |
| reporting| 20    |
| gateway  | 19    |

`pipelinesascode.tekton.dev/event-type`

| Value        | Count |
| ------------ | ----- |
| push         | 780   |
| retest       | 85    |
| pull_request | 71    |
| incoming     | 64    |

## UID ranges

Seed and live (store/mixed) streams draw UIDs from **disjoint** UUIDv5 ranges, so
a store or mixed run never collides with the loaded seed data. The ranges are
recorded in the definition's `uid_range` and in every report's metadata.
