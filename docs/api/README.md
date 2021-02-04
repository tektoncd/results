# Results API

## Authentication/Authorization

The reference implementation of the Results API expects
[cluster generated authentication tokens](https://kubernetes.io/docs/reference/access-authn-authz/authentication/)
from the cluster it is running on. In most cases, using a service account will
be the easiest way to interact with the API.

[RBAC Authorization](https://kubernetes.io/docs/reference/access-authn-authz/rbac/)
is used to control access to API Resources.

The following attributes are recognized:

| Attribute | Values                            |
| --------- | --------------------------------- |
| apiGroups | results.tekton.dev                |
| resources | results, records                  |
| verbs     | create, get, list, update, delete |

For example, a read-only Role might look like:

```
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: tekton-results-readonly
rules:
  - apiGroups: ["results.tekton.dev"]
    resources: ["results", "records"]
    verbs: ["get", "list"]
```

In the reference implementation, all permissions are scoped per namespace (this
is what is used as the API parent resource).

As a convenience, the following [ClusterRoles] are defined for common access
patterns:

| ClusterRole              | Description                                                                    |
| ------------------------ | ------------------------------------------------------------------------------ |
| tekton-results-readonly  | Read only access to all Result API resources                                   |
| tekton-results-readwrite | Includes `tekton-results-readonly` + Create or update all Result API resources |
| tekton-results-admin     | Includes `tekton-results-readwrite` + Allows deletion of Result API Resources  |

### Troubleshooting

The following command can be ran to query the cluster's permissions. This can be
useful for debugging permission denied errors:

```sh
$ kubectl create --as=system:serviceaccount:tekton-pipelines:tekton-results-watcher -n tekton-pipelines -f - -o yaml << EOF
apiVersion: authorization.k8s.io/v1
kind: SelfSubjectAccessReview
spec:
  resourceAttributes:
    group: results.tekton.dev
    resource: results
    verb: get
EOF
apiVersion: authorization.k8s.io/v1
kind: SelfSubjectAccessReview
metadata:
  creationTimestamp: null
  managedFields:
  - apiVersion: authorization.k8s.io/v1
    fieldsType: FieldsV1
    fieldsV1:
      f:spec:
        f:resourceAttributes:
          .: {}
          f:group: {}
          f:resource: {}
          f:verb: {}
    manager: kubectl
    operation: Update
    time: "2021-02-02T22:37:32Z"
spec:
  resourceAttributes:
    group: results.tekton.dev
    resource: results
    verb: get
status:
  allowed: true
  reason: 'RBAC: allowed by ClusterRoleBinding "tekton-results-watcher" of ClusterRole
    "tekton-results-watcher" to ServiceAccount "tekton-results-watcher/tekton-pipelines"'
```

## Filtering

The reference implementation of the Results API uses
[CEL](https://github.com/google/cel-spec/blob/master/doc/langdef.md) as a
filtering spec. Filter specs expect a boolean result value.

Known types exposed to each RPC method are documented below.

### ListResults

| Known Types | Description                                      |
| ----------- | ------------------------------------------------ |
| `result`    | [Result Object](/proto/v1alpha2/resources.proto) |

### ListRecords

| Known Types | Description                                      |
| ----------- | ------------------------------------------------ |
| `record`    | [Record Object](/proto/v1alpha2/resources.proto) |

#### Cookbook

| Filter Spec                                                                                                                                                                           | Description                                                                                      |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| `record.name.startsWith("foo/results/bar")`                                                                                                                                           | Get all Records belonging to Result `foo/results/bar`                                            |
| `type(record.data) == tekton.pipeline.v1beta1.TaskRun`                                                                                                                                | Get all Records of type TaskRun                                                                  |
| `type(record.data) == tekton.pipeline.v1beta1.TaskRun && record.data.metadata.name.contains("release") && record.data.spec.task_spec.steps.exists(step, step.name.contains("fetch"))` | Get TasksRuns with a name that contains "release" and at least 1 step name that contains "fetch" |

## Reading Records across Results

Records can be read across Results by specifying `-` as the Result name part
(e.g. `default/results/-`). This can be used to read and filter matching Records
without knowing the exact Result name.

## Metrics

The API Server includes an HTTP server for exposing gRPC server Prometheus metrics.
By default, the Service exposes metrics on port `:8080`. For more
details on the structure of the metrics, see
https://github.com/grpc-ecosystem/go-grpc-prometheus#metrics.
