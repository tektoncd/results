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

Users and/or integrators can pass arbitrary keys/values to Results by adding special annotations to PipelineRuns and TaskRuns:

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

## Supported version of TaskRun and PipelineRun CR

Results stores PipelineRun and TaskRun as v1. If there are older records, it's possible that they are stored as v1beta1. API server can be configured to start a converter during initialisation.

## Finalizer for blocking deletion

Watcher implements a finalizer to block deletion by an external pruner when logs are streamed via the Watcher.```

@khrm Please verify that's what you meant.

This requires that we pass `store_deadline` (default 10m). If deletion request comes, then it will block until completion + store_deadline is passed or resource is stored as record.
