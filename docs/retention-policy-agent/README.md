<!--

---
linkTitle: "Results Retention Policy Agent"
weight: 2
---

-->

# Result Retention Policy Agent

The Results Retention Policy Agent removes older Results and their associated Records from the DB.

## Configuration

The Results Retention Policy Agent is configured via the `tekton-results-config-results-retention-policy` ConfigMap.

The following fields are supported:

- `runAt`: Determines when to run the pruning job for the DB. It uses a cron schedule format. The default is `"7 7 * * 7"` (every Sunday at 7:07 AM).
- `maxRetention`: The default retention period for how long to keep Results and Records when no specific policy matches. This can be a number (e.g., `30`), which is interpreted as days, or a duration string (e.g., `30d`, `24h`). The default is `30d`.
- `policies`: A list of fine-grained retention policies that allow for more specific control over data retention.

### Fine-Grained Retention Policies

You can define a list of policies to control retention based on various criteria. The `policies` field in the ConfigMap accepts a YAML string containing a list of policy objects. Each policy has a `name`, a `selector`, and a `retention` period.

When the retention job runs, it evaluates a Result against the policies in the order they are defined. The **first policy that matches** the Result will be applied. If no policies match, the default `maxRetention` period is used.

#### Policy Fields:
- `name`: A descriptive name for the policy.
- `selector`: Defines the criteria for matching Results. All conditions within a selector must be met for the policy to match.
  - `matchNamespace`: A list of namespaces. A Result matches if its namespace is in this list.
  - `matchLabels`: A map where the key is a label name and the value is a list of possible label values. A Result matches if it has a label with the given name and its value is in the list.
  - `matchAnnotations`: A map where the key is an annotation name and the value is a list of possible annotation values. A Result matches if it has an annotation with the given name and its value is in the list.
  - `status`: A list of statuses (e.g., `Succeeded`, `Failed`, `Cancelled`). A Result matches if its final status is in this list.
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
  maxRetention: "30d"
  policies: |
    - name: "retain-critical-failures-long-term"
      selector:
        matchNamespace:
          - "production"
          - "prod-east"
        matchLabels:
          "criticality": ["high"]
        status:
          - "Failed"
      retention: "180d"
    - name: "retain-annotated-for-debug"
      selector:
        matchAnnotations:
          "debug/retain": ["true"]
      retention: "14d"
    - name: "default-production-policy"
      selector:
        matchNamespace:
          - "production"
          - "prod-east"
      retention: "60d"
    - name: "short-term-ci-retention"
      selector:
        matchNamespace:
          - "ci"
      retention: "7d"
```

In this example:
1.  A failed Result in the `production` or `prod-east` namespace with the label `criticality: high` will be kept for **180 days**.
2.  Any Result with the annotation `debug/retain: "true"` will be kept for **14 days**.
3.  Any other Result in the `production` or `prod-east` namespace will be kept for **60 days**.
4.  Any Result in the `ci` namespace will be kept for **7 days**.
5.  All other Results that do not match any of these policies will be kept for the default `maxRetention` period of **30 days**.