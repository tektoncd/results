<!--

---
linkTitle: "Results Watcher"
weight: 2
---

-->

# Result Watcher

The Result Watcher is a Kubernetes Controller that watches for changes to
certain Tekton types and automatically creates/updates their data in the Result
API.

## Supported Types
The Watcher currently supports the following types:

- `tekton.dev/v1beta1 TaskRun`
- `tekton.dev/v1beta1 PipelineRun`
- `tekton.dev/v1beta1 CustomRun`
- `tekton.dev/v1 TaskRun`
- `tekton.dev/v1 PipelineRun`

## Result Grouping

The Watcher uses Object data to automatically detect and group related Records
into the same Result. The following data is checked (listed in order of
precedence):

- `results.tekton.dev/result` annotation. This should correspond to the full
  `Result.name` identifier (e.g. `foo/results/bar`).
- `triggers.tekton.dev/triggers-eventid` label (this is generated from Objects
  created via [Tekton Triggers](https://github.com/tektoncd/triggers))
- An
  [OwnerReference](https://kubernetes.io/docs/concepts/workloads/controllers/garbage-collection/#owners-and-dependents)
  to a PipelineRun.

If no annotation is detected, the Watcher will automatically generate a new
Result name for the Object.

## Passing arbitrary key/values to Results

Users and/or integrators can pass arbitrary keys/values to Results by adding special annotations to PipelineRuns, TaskRuns, and CustomRuns:

- `results.tekton.dev/resultAnnotations`: a JSON object (string->string) to be stored into thee `Result.Annotations` field.
- `results.tekton.dev/recordSummaryAnnotations`: a JSON object (string->string) to be stored into thee `Result.Summary.Annotations` field.

Once the Watcher detects those annotations in the observed object, it passes the keys/values to the respective fields of the underlying Result. Those annotations can be used to store relevant metadata (e.g. the Git commit SHA that triggered a PipelineRun) into Results and may be used later to retrieve the objects from the API server. For instance:

```yaml
apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  generateName: hello-run-
  annotations:
    results.tekton.dev/resultAnnotations: |-
      {"repo": "tektoncd/results", "commit": "1a6b908"}
    results.tekton.dev/recordSummaryAnnotations: |-
      {"foo": "bar"}
```

## Resource Deletion

When the command line flag is `completed_run_grace_period` is set to any value other than `0`, resources will be deleted after the specified duration in the flag, calculated from the time of completion. If the value is < `0`, Runs will be deleted immediately after completion or failure.

The flag `check_owner` allows additional check before deleting a resource. If set `true`, resources with any owner references set will not be deleted. When the flag is `false`, owner references will be not be checked before deletion.

## Supported version of TaskRun, PipelineRun, and CustomRun CRs

Results stores PipelineRun and TaskRun as v1. CustomRun is stored as v1beta1 (the only version currently available in Tekton Pipelines). If there are older records, it's possible that they are stored as v1beta1. API server can be configured to start a converter during initialisation.

## Finalizer for blocking deletion

Watcher implements a finalizer to block deletion by an external pruner when objects are stored via the Watcher. Each resource type has its own finalizer (`results.tekton.dev/pipelinerun`, `results.tekton.dev/taskrun`, `results.tekton.dev/customrun`).

When deletion request comes, it will block until completion time + `completed_run_grace_period` period is passed. A hard limit could be set as `store_deadline` (default 10m), after which the object will be removed from the cluster even without confirmation it's been stored in the DB.

### Required annotations for finalizer release

The `--required_annotation` flag (repeatable) specifies annotations that must be present on a PipelineRun or TaskRun before the Watcher clears its finalizer. This is useful in multicluster scenarios where external controllers (e.g. a Hub scheduler or syncer service) need to write annotations on a resource before it can be safely deleted.

The flag supports two modes:
1. **Value matching:** Use `key=value` to require that the annotation exists AND its value exactly matches the provided value.
2. **Existence only:** Use `key` (without an `=`) to require that the annotation exists, regardless of what its value is.

The `results.tekton.dev/stored` annotation is always implicitly required and does not need to be listed. When the flag is not provided (the default), only the stored annotation is checked and behavior is unchanged from previous versions.

For example, to require that a Hub scheduler has annotated the resource (existence only) before the finalizer is cleared:

```
--required_annotation "hub.example.com/scheduled"
```

Multiple annotations with mixed requirements:

```
--required_annotation "hub.example.com/scheduled"
--required_annotation "ci.example.com/status=passed"
```

If any required annotation is missing or does not match its expected value, the Watcher re-queues the resource and checks again after the `FinalizerRequeueInterval` (10 seconds). The `store_deadline` safety limit still applies — if the deadline passes, the finalizer is cleared regardless of whether the required annotations are present.

> **Note:** This flag applies to both PipelineRun and TaskRun resources. CustomRuns are not affected.

## Filtering by `spec.managedBy`

The `managed_by_values` flag controls which TaskRuns and PipelineRuns the Watcher will process based on their `spec.managedBy` field. This is useful when multiple controllers manage Tekton resources and you want the Results Watcher to only track runs managed by specific controllers.

- `tekton.dev/pipeline` is **always accepted** and cannot be removed.
- Runs with **unset, empty, or whitespace-only** `spec.managedBy` are always accepted (backward compatible).
- Additional values can be specified as a comma-separated list.

For example, to also process runs managed by a custom controller:

```
--managed_by_values=custom-controller
```

Multiple values:

```
--managed_by_values=custom-controller,another-controller
```

CustomRuns are not filtered because the CustomRun type does not have a `spec.managedBy` field.

> **Note:** If a value is removed from `--managed_by_values`, the Watcher will
> still process runs that already carry a Results finalizer so the finalizer
> can be cleared and the resource can be deleted. New runs with that
> `managedBy` value will be ignored. Runs that had a finalizer but were not
> yet stored will **not** be stored — the finalizer is released without
> persisting data, matching the operator's intent to stop tracking those runs.

> **Known limitation:** Filtering applies to each resource independently.
> If an external controller sets `spec.managedBy` on a PipelineRun but its
> child TaskRuns or CustomRuns have nil `spec.managedBy` (the default), the
> Watcher will ignore the PipelineRun but still process the child runs.

## Disabling Incomplete Runs storage

The `disable_storing_incomplete_runs` flag controls whether the Watcher should store PipelineRuns, TaskRuns, and CustomRuns that are still in progress (i.e., not yet completed, cancelled or failed).

When set to `true`, the Watcher will only store Runs once they are completed. This is useful for reducing the load for API server and reconciliation queue. 

When set to `false` (default), the Watcher will attempt to continuously store all Runs on every modification regardless of their completion status, allowing you to track the full lifecycle of your PipelineRuns, TaskRuns, and CustomRuns.
