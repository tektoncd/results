<!--

---
linkTitle: "Results Retention Policy Agent"
weight: 2
---

-->

# Result Retention Policy Agent

The Results Retention Policy Agent removes older Results and their associated Records from the DB. The policies apply to `PipelineRun`, top-level `TaskRun`, and top-level `CustomRun` results.
Retention policies can be used to manage database size and performance, and the retention duration applies to the database records irrespective of their underlying Runs' age.

It is recommended that the Retention Policy Agent be used in conjunction with a cluster-resource pruning mechanism such as [Tekton Results Wacher's resource deletion](../watcher#resource-deletion) or [Tekton Pruner](https://github.com/tektoncd/pruner), with a Results Retention Policy longer than the in-cluster retention period.
This avoids the situation where a pruned Result record is re-created in the database because it still exists in the cluster.

For best results, the Retention Policy Agent should also be used in conjunction with the `disable_storing_incomplete_runs` [setting](../watcher#disabling-incomplete-runs-storage).

## Configuration

The Results Retention Policy Agent is configured via the `tekton-results-config-results-retention-policy` ConfigMap.

The following fields are supported:

- `runAt`: Determines when to run the pruning job for the DB. It uses a cron schedule format. The default is `"7 7 * * 7"` (every Sunday at 7:07 AM).
- `defaultRetention`: The **fallback** retention period for how long to store Results and Records when no specific policy matches. This value does **not** override the retention period of a matching policy; it only applies when no policies match a given Result. This can be a number (e.g., `30`), which is interpreted as days, or a duration string (e.g., `30d`, `24h`). The default is `30d`.

> **⚠️ IMPORTANT - Migration from `maxRetention` to `defaultRetention`. DATA LOSS RISK**
> 
> `maxRetention` is **deprecated** and will be removed in a future release. Please migrate to `defaultRetention` as soon as possible. If a user has `maxRetention` set higher than 30 days and does not migrate to `defaultRetention`, when `maxRetention` is removed the `defaultRetention` records older than the default `defaultRetention` may be deleted.
> 
> To migrate: modify the `tekton-results-config-results-retention-policy` ConfigMap to rename `data.maxRetention` to `data.defaultRetention`.
> 
> **Backward Compatibility Behavior:**
> - If both `maxRetention` and `defaultRetention` are present, `maxRetention` takes priority to maintain backward compatibility.
> - If only `maxRetention` is set, it will be used (with a deprecation warning in logs).
> - If only `defaultRetention` is set, it will be used (recommended).

- `policies`: A list of fine-grained retention policies that allow for more specific control over data retention.
- `namespaceCleanup`: Configuration of the removal of the data belonging to namespaces that no longer exist in the cluster. Disabled by default. See [Namespace Cleanup](#namespace-cleanup).

### Fine-Grained Retention Policies

You can define a list of policies to control retention based on various criteria. The `policies` field in the ConfigMap accepts a YAML string containing a list of policy objects. Each policy has a `name`, a `selector`, and a `retention` period.

When the retention job runs, it evaluates a Result against the policies in the order they are defined. The **first policy that matches** the Result will be applied. If no policies match, the default `defaultRetention` period is used.

#### Policy Fields:
- `name`: A descriptive name for the policy.
- `selector`: Defines the criteria for matching Results. All conditions within a selector are combined with an **AND** logic—a Result must meet all specified criteria (`matchNamespaces`, `matchLabels`, `matchAnnotations`, `matchStatuses`) for the policy to apply. If a particular selector type (e.g., `matchLabels`) is omitted from a policy, it will match all Results for that criterion. For example, a policy without a `matchNamespaces` selector will match Results from any namespace.
  - `matchNamespaces`: A list of namespaces. A Result matches if its namespace is in this list (an **OR** logic is applied to the values in the list).
  - `matchLabels`: A map where the key is a label name and the value is a list of possible label values. A Result must have all the specified label keys, and for each key, its value must be in the provided list (an **OR** logic is applied to the values in the list).
  - `matchAnnotations`: A map where the key is an annotation name and the value is a list of possible annotation values. This works similarly to `matchLabels`.
  - `matchStatuses`: A list of final statuses. A Result matches if its final status is in this list (an **OR** logic is applied to the values in the list). The status is determined by the `reason` field of the primary `Succeeded` condition in the `PipelineRun` or `TaskRun` status. Common values include `Succeeded`, `Failed`, `Cancelled`, `Running`, and `Pending`. For a more comprehensive list of possible status reasons, refer to the [Tekton documentation](https://tekton.dev/docs/pipelines/pipelineruns/#monitoring-execution-status).
- `retention`: The retention period for Results matching this policy. This can be a number (e.g., `7`), which is interpreted as days, or a duration string (e.g., `24h`).

#### Example ConfigMap:

Here is an example of a `ConfigMap` that defines multiple, comprehensive retention policies:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: tekton-results-config-results-retention-policy
  namespace: tekton-pipelines
data:
  runAt: "0 2 * * *" # Run every day at 2:00 AM
  defaultRetention: "30d"
  policies: |
    - name: "retain-critical-failures-long-term"
      selector:
        matchNamespaces:
          - "production"
          - "prod-east"
        matchLabels:
          "criticality": ["high"]
        matchStatuses:
          - "Failed"
      retention: "180d"
    - name: "retain-annotated-for-debug"
      selector:
        matchAnnotations:
          "debug/retain": ["true"]
      retention: "14d"
    - name: "default-production-policy"
      selector:
        matchNamespaces:
          - "production"
          - "prod-east"
      retention: "60d"
    - name: "short-term-ci-retention"
      selector:
        matchNamespaces:
          - "ci"
      retention: "7d"
```

In this example:
1.  A failed Result in the `production` or `prod-east` namespace with the label `criticality: high` will be kept in the database for **180 days**.
2.  Any Result with the annotation `debug/retain: "true"` will be kept for **14 days**.
3.  Any other Result in the `production` or `prod-east` namespace will be kept for **60 days**.
4.  Any Result in the `ci` namespace will be kept for **7 days**.
5.  All other Results that do not match any of these policies will be kept for the default `defaultRetention` period of **30 days**.

## Namespace Cleanup

When a namespace is deleted from the cluster, the Results and Records it produced remain in the database until their retention period expires. The optional namespace cleanup removes that data as soon as the namespace is gone, without waiting for the retention period.

Namespace cleanup is **disabled by default** and is configured under the `namespaceCleanup` key of the `tekton-results-config-results-retention-policy` ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: tekton-results-config-results-retention-policy
  namespace: tekton-pipelines
data:
  runAt: "0 2 * * *"
  defaultRetention: "30d"
  namespaceCleanup: |
    enabled: true
    inactivityPeriod: "24h"
    excludeNamespaces:
      - "kube-system"
    maxNamespacesPerRun: 10
    dryRun: false
```

### Fields

- `enabled`: Turns the namespace cleanup on. The default is `false`.
- `inactivityPeriod`: A namespace is only eligible for cleanup once its data has not been updated for at least this long. This protects the data of namespaces in the case of accidental removal. This can be a number (e.g., `2`), which is interpreted as days, or a duration string (e.g., `24h`, `2d`). The default is `24h`.
- `excludeNamespaces`: A list of namespaces whose data is kept even when they no longer exist in the cluster. The exclusion only applies to the namespace cleanup. The data of those namespaces is still subject to `defaultRetention` and to any matching policy, so it is removed once it exceeds its retention period. The default is empty.
- `maxNamespacesPerRun`: The maximum number of namespaces removed in a single run. If more namespaces are eligible, **nothing is deleted** and the run is skipped. This acts as a safety valve against an unexpected mass deletion. The default is `10`. It must be greater than `0`.

  A skipped run is reported at `error` level in the Retention Policy Agent logs, listing the eligible namespaces and the setting to raise, for example:

  ```
  namespace cleanup skipped and no data was deleted: 14 namespaces are eligible for cleanup but maxNamespacesPerRun is 10. Review the namespaces below and, if the cleanup is expected, raise namespaceCleanup.maxNamespacesPerRun in the tekton-results-config-results-retention-policy ConfigMap, or add the namespaces to namespaceCleanup.excludeNamespaces to keep their data. Eligible namespaces: ...
  ```

  The cleanup stays skipped on every subsequent run until an administrator raises `maxNamespacesPerRun` or excludes the namespaces, so it is worth alerting on `error` level logs of the agent.

  At most **25 namespace names are listed** in a log message, to keep the entry readable. When more namespaces are eligible, the message ends with `and N more`, so the reported count at the beginning of the message, not the listed names, reflects how many namespaces are actually eligible. The same limit applies to the `dryRun` message.
- `dryRun`: When `true`, the namespaces that would be cleaned up are logged and no data is deleted. Useful to review the impact before enabling the cleanup. The default is `false`. As above, at most 25 namespace names are listed in the log message, followed by `and N more` when there are others.

### Behavior

The cleanup runs as part of the retention job, on the schedule defined by `runAt`. On each run the agent:

1. Lists the namespaces of the cluster. If the list cannot be retrieved, or the cluster reports no namespaces at all, the cleanup is skipped and nothing is deleted.
2. Selects the namespaces stored in the database whose data has not been updated within the `inactivityPeriod`.
3. Removes from that set the namespaces that still exist in the cluster and the ones listed in `excludeNamespaces`.
4. Deletes the Results, and their associated Records, of the remaining namespaces.

> **⚠️ IMPORTANT - DATA LOSS RISK**
>
> Namespace cleanup ignores the retention periods. Once a namespace is deleted from the cluster and its data has been inactive for the `inactivityPeriod`, **all** of its data is removed from the database, regardless of the `defaultRetention` or of any matching policy. Enable `dryRun` first to review what would be deleted.

> **Note**
>
> Namespace cleanup only removes the data stored in the database. Logs stored in an external logging backend, such as Loki, Blob storage or Splunk, are **not** removed. This is expected to be addressed in a future release.

### Permissions

The Retention Policy Agent must be able to list the namespaces of the cluster. This permission is part of the `tekton-results-retention-policy-agent` ClusterRole shipped with the release:

```yaml
rules:
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["list"]
```

If the agent runs with a custom ServiceAccount, grant it the same permission, otherwise the cleanup is skipped on every run and an error is logged.

## Migrating from `maxRetention` to `defaultRetention`

In the `tekton-results-config-results-retention-policy` ConfigMap, rename `data.maxRetention` to `data.defaultRetention`.
