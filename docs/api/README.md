<!--

---
linkTitle: "Results API"
weight: 2
---

-->

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

```yaml
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

### Impersonation

[Kubernetes' impersonation](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#user-impersonation)
is used to refine access to the APIs.

#### How to use Impersonation?

Run API server with feature flag `AUTH_IMPERSONATE` set to `true` in the config.

Create two `ServiceAccount`, one for administering the permissions to impersonate and the other one  for the 
permissions of the user to impersonate.

```shell
kubectl create serviceaccount impersonate-admin -n tekton-pipelines
```
```shell
kubectl create serviceaccount impersonate-user -n user-namespace
```

Create the following `ClusterRole` and `ClusterRoleBinding` for the `impersonate-admin` service account.
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: tekton-results-impersonate
rules:
  - apiGroups: [""]
    resources: ["users", "groups", "serviceaccounts"]
    verbs: ["impersonate"]
  - apiGroups: ["authentication.k8s.io"]
    resources: ["uids"]
    verbs: ["impersonate"]
```
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: tekton-results-impersonate
subjects:
  - kind: ServiceAccount
    name: impersonate-admin
    namespace: tekton-pipelines
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: tekton-results-impersonate
```
Create `RoleBinding` for `impersonate-user` service account.
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: tekton-results-user
  namespace: user-namespace
subjects:
  - kind: ServiceAccount
    name: impersonate-user
    namespace: user-namespace
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: tekton-results-readonly
```
Now get a token for service account `impersonate-admin` and store in a variable.
```shell
token=$(kubectl create token impersonate-admin -n tekton-pipelines)
```
Then the APIs can be called in the following format
```shell
curl -s --cacert /var/tmp/tekton/ssl/tls.crt  \
-H 'authorization: Bearer '${token} \
-H 'Impersonate-User: system:serviceaccount:user-namespace:impersonate-user' \
https://localhost:8080/apis/results.tekton.dev/v1alpha2/parents/user-namespace/results
```
Need to provide a TLS cert if API server is using TLS.

### Troubleshooting

The following command can be run to query the cluster's permissions. This can be
useful for debugging permission denied errors:

```sh
kubectl create --as=system:serviceaccount:tekton-pipelines:tekton-results-watcher -n tekton-pipelines -f - -o yaml << EOF
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

### Filter Fields

#### Records

| Name      | Description                                                                                                                                              |
| --------- |----------------------------------------------------------------------------------------------------------------------------------------------------------|
| `name`      | Record name                                                                                                                                              |
| `data_type` | Type identifier of the Record data (corresponds to [Any.type](../../proto/v1alpha2/resources.proto))                                                     |
| `data`      | Record data (see [JSON Data Conversion](https://github.com/google/cel-spec/blob/master/doc/langdef.md#json-data-conversion) for how CEL represents this) |

### Cookbook

| Filter Spec                                                                    | Description                                                                  |
| ------------------------------------------------------------------------------ | ---------------------------------------------------------------------------- |
| `name.startsWith("foo/results/bar")`                                           | Get all Records belonging to Result `foo/results/bar`                        |
| `data_type == "tekton.dev/v1beta1.TaskRun"`                                    | Get all Records of type TaskRun                                              |
| `data.status.conditions.has(c, c.type=="Succeeded" && c.status=="False")`      | Get all TaskRuns and PipelineRuns that have failed.                          |
| `data.status.completion_time - record.data.status.start_time > duration("5m")` | Get all TaskRuns and PipelineRuns that took more than 5 minutes to complete. |

**NOTE**: While performing a REST request, CEL filtering expressions
should be passed by specifying `filer=<cel-expression>` format. Examples:
`<query-url>?filter=name.startsWith("foo/results/bar")` or `<query-url>?filter=data_type=="results.tekton.dev/v1alpha2.Log`.

## Ordering

The reference implementation of the Results API supports ordering result and
record responses with an optional direction qualifier (either `asc` or `desc`).

To request a list of objects with a specific order include an `order_by` query
parameter in your request. Pass it the name of the field to be ordered on.
Multiple fields can be specified with a comma-separated list. Examples:

- `created_time`
- `updated_time asc`
- `created_time desc, updated_time asc`

Fields supported in `order_by`:

| Field Name     |
| -------------- |
| `created_time` |
| `updated_time` |

## Pagination

The reference implementation of Results API supports pagination for results, records
and logs. The default number of objects in a single page is 50 and the
maximum number is 10000.

To paginate the response, include `page_size` query parameter in your request.
It must be an integer value between 0 and 10000. If the `page_size` is less than
the number of total objects available for the particular query, the response will
include a `NextPageToken` in the response. You can pass this value to `page_token`
query parameter to fetch the next page. Both the queries are independent and can
be used individually or together.

| Name | Description |
| `page_size` | The number of objects to fetch in the response. |
| `page_token` | Token of the page to be fetched. |

## Reading results across parents

Results can be read across parents by specifying `-` as the parent name. This is useful for listing all results stored in the system without a prior knowledge about the available parents.

## Reading Records across Results

Records can be read across Results by specifying `-` as the Result name part or across parents by specifying `-` as the parent name.
(e.g. `default/results/-` or `-/results/-`). This can be used to read and filter matching Records
without knowing the exact Result name.

## Metrics

The API Server includes an HTTP server for exposing gRPC server Prometheus
metrics. By default, the Service exposes metrics on port `:9090`. For more
details on the structure of the metrics, see
<https://github.com/grpc-ecosystem/go-grpc-prometheus#metrics>.

## References

- [OpenAPI Specification](openapi.yaml)
