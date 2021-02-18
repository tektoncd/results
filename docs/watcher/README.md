<!--
---
title: "Results Watcher"
linkTitle: "Results Watcher"
weight: 2
---
-->
# Result Watcher

The Result Watcher is a Kubernetes Controller that watches for changes to certain Tekton types and automatically creates/updates their data in the Result API.

## Supported Types

The Watcher currently supports the following types:

- `tekton.dev/v1beta1 TaskRun`
- `tekton.dev/v1beta1 PipelineRun`

## Result Grouping

The Watcher uses Object data to automatically detect and group related Records into the same Result. The following data is checked (listed in order of precedence):

- `results.tekton.dev/result` annotation. This should correspond to the full `Result.name` identifier (e.g. `foo/results/bar`).
- `triggers.tekton.dev/triggers-eventid` label (this is generated from Objects created via [Tekton Triggers](https://github.com/tektoncd/triggers))
- An [OwnerReference](https://kubernetes.io/docs/concepts/workloads/controllers/garbage-collection/#owners-and-dependents) to a PipelineRun.

If no annotation is detected, the Watcher will automatically generate a new Result name for the Object.