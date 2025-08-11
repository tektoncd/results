<!--

---
linkTitle: "Results Retention Policy Agent"
weight: 2
---

-->

# Result Retention Policy Agent

The Results Retention Policy Agent removes older Results and their associated Records from the DB. The policies apply to both `PipelineRun` and top-level `TaskRun` results.

## Configuration

The Results Retention Policy Agent is configured via the `tekton-results-config-results-retention-policy` ConfigMap.

The following fields are supported:

- `runAt`: Determines when to run the pruning job for the DB. It uses a cron schedule format. The default is `"7 7 * * 7"` (every Sunday at 7:07 AM).
- `defaultRetention`: The **fallback** retention period for how long to keep Results and Records when no specific policy matches. This value does **not** override the retention period of a matching policy; it only applies when no policies match a given Result. This can be a number (e.g., `30`), which is interpreted as days, or a duration string (e.g., `30d`, `24h`). The default is `30d`.
> **Note:**
> `maxRetention` is deprecated and will be removed in a future release. If `defaultRetention` is not set, `maxRetention` will be used as the default retention period for backward compatibility.
- `policies`: A list of fine-grained retention policies that allow for more specific control over data retention.

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
1.  A failed Result in the `production` or `prod-east` namespace with the label `criticality: high` will be kept for **180 days**.
2.  Any Result with the annotation `debug/retain: "true"` will be kept for **14 days**.
3.  Any other Result in the `production` or `prod-east` namespace will be kept for **60 days**.
4.  Any Result in the `ci` namespace will be kept for **7 days**.
5.  All other Results that do not match any of these policies will be kept for the default `defaultRetention` period of **30 days**.