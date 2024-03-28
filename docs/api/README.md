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

- Run API server with feature flag `AUTH_IMPERSONATE` set to `true` in the config.

- Create two `ServiceAccount`, one for administering the permissions to impersonate and the other one
  for the permissions of the user to impersonate.

  ```shell
  kubectl create serviceaccount impersonate-admin -n tekton-pipelines
  ```

  ```shell
  kubectl create serviceaccount impersonate-user -n user-namespace
  ```

- Create the following `ClusterRole` and `ClusterRoleBinding` for the `impersonate-admin` service account.

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

- Finally, create `RoleBinding` for `impersonate-user` service account.

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

- Now get a token for the service account `impersonate-admin` and store in a variable.

  ```shell
  token=$(kubectl create token impersonate-admin -n tekton-pipelines)
  ```

- Then the APIs can be called in the following format

  ```shell
  curl -s --cacert /var/tmp/tekton/ssl/tls.crt  \
    -H 'authorization: Bearer '${token} \
    -H 'Impersonate-User: system:serviceaccount:user-namespace:impersonate-user' \
    https://localhost:8080/apis/results.tekton.dev/v1alpha2/parents/user-namespace/results
  ```

Need to provide a TLS cert if the API server is using TLS.

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
filtering spec. Filter specs expect a boolean result value. This document covers
a small subset of CEL useful for filtering Results and Records.

### Result

Here is the mapping between the Result JSON/protobuf fields and the CEL references:

| Field         | CEL Reference Field | Description                                       |
| ------------- | ------------------- | ------------------------------------------------- |
| -             | `parent`            | Parent (workspace/namespace) name for the Result. |
| `uid`         | `uid`               | Unique identifier for the Result.                 |
| `annotations` | `annotations`       | Annotations added to the Result.                  |
| `summary`     | `summary`           | The summary of the Result.                        |
| `createTime`  | `create_time`       | The creation time of the Result.                  |
| `updateTime`  | `update_time`       | The last update time of the Result.               |

The `summary.status` field is an enum and must be used in filtering expression
without quotes (`'` or `"`). Possible values are:

- UNKNOWN
- SUCCESS
- FAILURE
- TIMEOUT
- CANCELLED

### Record and Log

Here is the mapping between the Record JSON/protobuf fields and the CEL references:

| Field        | CEL Reference Field | Description                                                                                                           |
| ------------ | ------------------- | --------------------------------------------------------------------------------------------------------------------- |
| `name`       | `name`              | Record name                                                                                                           |
| `data.type`  | `data_type`         | It is the type identifier of the Record data. See below for values.                                                   |
| `data.value` | `data`              | It is the data contained by the Record. In JSON and protobuf response, it is represented as a base 64 encoded string. |

Possible values for `data_type` and `summary.type` (for Result) are:

- `tekton.dev/v1beta1.TaskRun` or `TASK_RUN`
- `tekton.dev/v1beta1.PipelineRun` or `PIPELINE_RUN`
- `results.tekton.dev/v1alpha2.Log`

#### The `data` field in Record

The `data` field is the base64 encoded string of the manifest of the object. If
you directly request this data using CLI, REST or gRPC you will get the base64
encoded string. You can decode it using `base64 -d` command. This is not human-readable,
but you can directly use filters to filter the response without need to decode it.

Here is an example of the JSON object contained in the `data` field of a Record.
This is directly mappable to the YAML notation we generally use.

```json
{
  "kind": "PipelineRun",
  "spec": {
    "timeout": "1h0m0s",
    "pipelineSpec": {
      "tasks": [
        {
          "name": "hello",
          "taskSpec": {
            "spec": null,
            "steps": [
              {
                "name": "hello",
                "image": "ubuntu",
                "script": "echo hello world!",
                "resources": {}
              }
            ],
            "metadata": {}
          }
        }
      ]
    },
    "serviceAccountName": "default"
  },
  "status": {
    "startTime": "2023-08-22T09:08:59Z",
    "conditions": [
      {
        "type": "Succeeded",
        "reason": "Succeeded",
        "status": "True",
        "message": "Tasks Completed: 1 (Failed: 0, Cancelled 0), Skipped: 0",
        "lastTransitionTime": "2023-08-22T09:09:31Z"
      }
    ],
    "pipelineSpec": {
      "tasks": [
        {
          "name": "hello",
          "taskSpec": {
            "spec": null,
            "steps": [
              {
                "name": "hello",
                "image": "ubuntu",
                "script": "echo hello world!",
                "resources": {}
              }
            ],
            "metadata": {}
          }
        }
      ]
    },
    "completionTime": "2023-08-22T09:09:31Z",
    "childReferences": [
      {
        "kind": "TaskRun",
        "name": "hello-hello",
        "apiVersion": "tekton.dev/v1beta1",
        "pipelineTaskName": "hello"
      }
    ]
  },
  "metadata": {
    "uid": "1638b693-844d-4f13-b767-d7d84ac4ab3d",
    "name": "hello",
    "labels": {
      "tekton.dev/pipeline": "hello"
    },
    "namespace": "default",
    "generation": 1,
    "annotations": {
      "results.tekton.dev/record": "default/results/1638b693-844d-4f13-b767-d7d84ac4ab3d/records/1638b693-844d-4f13-b767-d7d84ac4ab3d",
      "results.tekton.dev/result": "default/results/1638b693-844d-4f13-b767-d7d84ac4ab3d",
      "results.tekton.dev/resultAnnotations": "{\"repo\": \"tektoncd/results\", \"commit\": \"1a6b908\"}",
      "results.tekton.dev/recordSummaryAnnotations": "{\"foo\": \"bar\"}",
      "kubectl.kubernetes.io/last-applied-configuration": "{\"apiVersion\":\"tekton.dev/v1beta1\",\"kind\":\"PipelineRun\",\"metadata\":{\"annotations\":{\"results.tekton.dev/recordSummaryAnnotations\":\"{\\\"foo\\\": \\\"bar\\\"}\",\"results.tekton.dev/resultAnnotations\":\"{\\\"repo\\\": \\\"tektoncd/results\\\", \\\"commit\\\": \\\"1a6b908\\\"}\"},\"name\":\"hello\",\"namespace\":\"default\"},\"spec\":{\"pipelineSpec\":{\"tasks\":[{\"name\":\"hello\",\"taskSpec\":{\"steps\":[{\"image\":\"ubuntu\",\"name\":\"hello\",\"script\":\"echo hello world!\"}]}}]}}}\n"
    },
    "managedFields": [
      {
        "time": "2023-08-22T09:08:59Z",
        "manager": "controller",
        "fieldsV1": {
          "f:metadata": {
            "f:labels": {
              ".": {},
              "f:tekton.dev/pipeline": {}
            }
          }
        },
        "operation": "Update",
        "apiVersion": "tekton.dev/v1beta1",
        "fieldsType": "FieldsV1"
      },
      {
        "time": "2023-08-22T09:08:59Z",
        "manager": "kubectl-client-side-apply",
        "fieldsV1": {
          "f:spec": {
            ".": {},
            "f:pipelineSpec": {
              ".": {},
              "f:tasks": {}
            }
          },
          "f:metadata": {
            "f:annotations": {
              ".": {},
              "f:results.tekton.dev/resultAnnotations": {},
              "f:results.tekton.dev/recordSummaryAnnotations": {},
              "f:kubectl.kubernetes.io/last-applied-configuration": {}
            }
          }
        },
        "operation": "Update",
        "apiVersion": "tekton.dev/v1beta1",
        "fieldsType": "FieldsV1"
      },
      {
        "time": "2023-08-22T09:08:59Z",
        "manager": "watcher",
        "fieldsV1": {
          "f:metadata": {
            "f:annotations": {
              "f:results.tekton.dev/record": {},
              "f:results.tekton.dev/result": {}
            }
          }
        },
        "operation": "Update",
        "apiVersion": "tekton.dev/v1beta1",
        "fieldsType": "FieldsV1"
      },
      {
        "time": "2023-08-22T09:09:31Z",
        "manager": "controller",
        "fieldsV1": {
          "f:status": {
            ".": {},
            "f:startTime": {},
            "f:conditions": {},
            "f:pipelineSpec": {
              ".": {},
              "f:tasks": {}
            },
            "f:completionTime": {},
            "f:childReferences": {}
          }
        },
        "operation": "Update",
        "apiVersion": "tekton.dev/v1beta1",
        "fieldsType": "FieldsV1",
        "subresource": "status"
      }
    ],
    "resourceVersion": "1567",
    "creationTimestamp": "2023-08-22T09:08:59Z"
  },
  "apiVersion": "tekton.dev/v1beta1"
}
```

You can now access the required fields using the dot notation and considering `data`
as the parent object. For example:

| Purpose                                                 | Filter Expression                      |
| ------------------------------------------------------- | -------------------------------------- |
| Name of the PipelineRun                                 | `data.metadata.name`                   |
| Name of the ServiceAccount used in the PipelineRun      | `data.spec.serviceAccountName`         |
| Name of the first task of the PipelineRun from its spec | `data.spec.pipelineSpec.tasks[0].name` |
| Start time of the PipelineRun                           | `data.status.startTime`                |
| Labels of the PipelineRun                               | `data.metadata.labels`                 |
| Annotations of the PipelineRun                          | `data.metadata.annotations`            |
| A particular annotation of the PipelineRun              | `data.metadata.annotations['foo']`     |

#### The `data` field in Log

The `data` field is the base64 encoded custom object of the type `Log`. Accessing
the fields is similar to records. Given below is an example of the JSON object
contained in the `data` field of a Log.

```json
{
  "kind": "Log",
  "spec": {
    "type": "File",
    "resource": {
      "uid": "dbe14a60-1fc8-458e-a49b-264771557c3e",
      "kind": "TaskRun",
      "name": "hello",
      "namespace": "default"
    }
  },
  "status": {
    "path": "default/2d27dde5-e201-35f9-b658-2456f2955903/hello-log",
    "size": 83
  },
  "metadata": {
    "uid": "2d27dde5-e201-35f9-b658-2456f2955903",
    "name": "hello-log",
    "namespace": "default",
    "creationTimestamp": null
  },
  "apiVersion": "results.tekton.dev/v1alpha2"
}
```

Here are some examples of accessing the fields of the Log using the dot notation:

| Purpose                               | Filter Expression         |
| ------------------------------------- | ------------------------- |
| Type of the Run that created this Log | `data.spec.resource.kind` |
| Size of the Log                       | `data.status.size`        |
| Name of the Run that created this Log | `data.spec.resource.name` |

### How to Create CEL Filtering Expressions

CEL expressions are composed of identifiers, literals, operators, and functions.
Here, we will learn how to create CEL filtering expressions using the above-mentioned fields.
This is not an exhaustive list of CEL expressions. For more information, please refer to
[CEL Specification](https://github.com/google/cel-spec/blob/master/doc/langdef.md).

#### Accessing Fields

The CEL expression generally has one-to-one mapping with the JSON/protobuf fields.
In Tekton Results, we have created some extra alias for easy access. You can see
all of them in the tables above. Other than that, you can access any field of the
JSON/protobuf object using the dot notation. See the examples in the tables below.

| Purpose                                                       | Filter Expression                    | Description                                                          |
| ------------------------------------------------------------- | ------------------------------------ | -------------------------------------------------------------------- |
| Status of a Result                                            | `summary.status`                     | The `status` of a Result is a child of the `summary`` object.        |
| Data type of Record                                           | `data_type`                          | `data_type` is a defined alias for `data.type` for a Record.         |
| Data type of Result                                           | `summary.type`                       | The `type` of a Result is a child of `summary``.                     |
| Name of the first step of the tasks of a run from its status. | `data.status.taskSpec.steps[0].name` | The JSON path is `data -> status -> taskSpec -> steps -> 0 -> name`. |

#### Using Operators

Now that we can access the fields, you can create a filter using operators. Here
is a list of operators that can be used in CEL expressions:

| Operator                | Description          | Example                                                                                        |
| ----------------------- | -------------------- | ---------------------------------------------------------------------------------------------- |
| `==`                    | Equal to             | `data_type == "tekton.dev/v1beta1.TaskRun"`                                                    |
| `!=`                    | Not equal to         | `summary.status != SUCCESS`                                                                  |
| `IN`                    | In a list            | `data.metadata.name in ['hello', 'foo', 'bar']`                                                |
| `!`                     | Negation             | `!(data.status.name in ['hello', 'foo', 'bar'])`                                               |
| `&&`                    | Logical AND          | `data_type == "tekton.dev/v1beta1.TaskRun" && name.startsWith("foo/results/bar")`              |
| `\|\|`                  | Logical OR           | `data_type == "tekton.dev/v1beta1.TaskRun" \|\| data_type == "tekton.dev/v1beta1.PipelineRun"` |
| `+`, `-`, `*`, `/`, `%` | Arithmetic operators | `data.status.completionTime - data.status.startTime > duration('5m')`                          |
| `>`, `>=`, `<`, `<=`    | Comparison operators | `data.status.completionTime > data.status.startTime`                                           |

#### Using Functions

There are many functions that can be used in CEL expressions. Here is a
list of functions that can be used in CEL expressions. The string in the function
argument shows the expected type of the argument:

| Functions                                             | Description                                                       | Example                                                        |
| ----------------------------------------------------- | ----------------------------------------------------------------- | -------------------------------------------------------------- |
| `startsWith('string')`                                | Checks if a string starts with a prefix                           | `data.metadata.name.startsWith("foo")`                         |
| `endsWith('string')`                                  | Checks if a string ends with a suffix                             | `data.metadata.name.endsWith("bar")`                           |
| `contains('string')`                                  | Checks if a field is present or an object contains a key or value | `data.metadata.annotations.contains('bar')`                    |
| `timestamp('RFC3339-timestamp')`                      | Used for comparing timestamps                                     | `data.status.startTime > timestamp("2021-02-02T22:37:32Z")`    |
| `getDate()`                                           | Returns the date from a timestamp                                 | `data.status.completionTime.getDate() == 7`                    |
| `getDayOfWeek()`, `getDayOfMonth()`, `getDayOfYear()` | Returns the day of the week, month or year from a timestamp       | `data.status.completionTime.getDayOfMonth() == 7`              |
| `getFullYear()`                                       | Returns the year from a timestamp                                 | `data.status.startTime.getFullYear() == 2023`                  |
| `getHours()`, `getMinutes()`, `getSeconds()`          | Returns the hours, minutes or seconds from a timestamp            | `data.status.completionTime.getHours() >= 9`                   |
| `string('input')`                                     | Convert a valid input to string                                   | `string(data.status.completionTime) == "2021-02-02T22:37:32Z"` |
| `matches('regex')`                                    | Checks if a string matches a regex                                | `name.matches("^foo.*$")`                                      |

You can also nest the function calls and mix operators to create complex filtering
expressions. Make sure to use the correct type of the argument for the function.
You can see a more exhaustive reference of functions in the CEL specification.
The functions mentioned above are the ones that are most commonly used and working.

### Using CEL Filtering Expressions with gRPC

You can pass filters to gRPC requests by specifying `filter=<cel-expression>`.
Please enclose the queries in proper quotes or use `\` if needed. See the examples below:

```bash
grpc_cli call --channel_creds_type=ssl \
  --ssl_target=tekton-results-api-service.tekton-pipelines.svc.cluster.local \
  --call_creds=access_token=$ACCESS_TOKEN \
  localhost:8080 tekton.results.v1alpha2.Results.ListResults \
  'parent:"default",filter:"data_type==TASK_RUN"'
```

### Using CEL Filtering Expressions with REST

You can pass filters to REST requests by specifying `filter=<cel-expression>` in the query.
See the examples below:

```bash
curl --insecure
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Accept: application/json" \
  https://localhost:8080/apis/results.tekton.dev/v1alpha2/parents/-/results/-?filter=data.status.completionTime.getDate()==7
```

### Using CEL Filtering Expressions with `tkn-results`

If you have the `tkn-results` CLI installed either independently or as a plugin for `tkn`,
you can use the `--filter=<cel-expression>` flag to filter the results. See the examples below:

```bash
tkn results records list default/results/- --filter="data.metadata.annotations.contains('bar')"
```

### Commonly used filters examples

These example shows the most used filtering expression that would be useful for
everyday use. Keep in mind that not all of these filters are available for Results, Records and Logs.
You must be providing the correct filter for the correct resource.

| Purpose                                                                                                   | Filter Expression                                                                                                  |
| --------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| Get all Records where the TaskRun/PipelineRun name is `hello`                                             | `data.metadata.name == 'hello'`                                                                                    |
| Get all Records of TaskRuns which are part of the PipelineRun 'foo'                                       | `data.metadata.labels['tekton.dev/pipelineRun'] == 'foo'`                                                          |
| Get all the Records of the TaskRun/PipelineRun which are part of Pipeline 'bar'                           | `data.metadata.labels['tekton.dev/pipeline'] == 'bar'`                                                             |
| Same query as above, but I only want the PipelineRuns                                                     | `data.metadata.labels['tekton.dev/pipeline'] == 'bar' && data_type == 'PIPELINE_RUN'`                              |
| Get the Records of the TaskRuns whose name starts with `hello`                                            | `data.metadata.name.startsWith('hello')&&dat_type==TASK_RUN`                                                       |
| Get all the Results of Successful TaskRuns                                                                | `summary.status == SUCCESS && summary.type == 'TASK_RUN'`                                                        |
| Get the Records of the PipelineRuns whose completion time is greater than 5 minutes                       | `data.status.completionTime - data.status.startTime > duration('5m') && data_type == 'PIPELINE_RUN'`               |
| Get the Records of the Runs which completed today (let's assume today is 7th)                             | `data.status.completionTime.getDate() == 7`                                                                        |
| Get the Records of the PipelineRuns which has annotations containing `bar`                                | `data.metadata.annotations.contains('bar') && data_type == 'PIPELINE_RUN'`                                         |
| Get the Records of the PipelineRuns which has annotations containing `bar` and the name starts with `foo` | `data.metadata.annotations.contains('bar') && data.metadata.name.startsWith('foo') && data_type == 'PIPELINE_RUN'` |
| Get the Results containing the annotations `foo` and `bar`                                                | `summary.annotations.contains('foo') && summary.annotations.contains('bar')`                                       |
| Get the Results of all the Runs that failed                                                               | `!(summary.status == SUCCESS)`                                                                                   |
| Get all the Records of the Runs that failed                                                               | `!(data.status.conditions[0].status == 'True')`                                                                    |
| Get all the Records of the PipelineRuns which had 3 or more tasks                                         | `size(data.status.pipelineSpec.tasks) >= 3 && data_type == 'PIPELINE_RUN'`                                         |

## Ordering

The reference implementation of the Results API supports ordering result and
record responses with an optional direction qualifier (either `asc` or `desc`).

To request a list of objects with a specific order, include a `order_by` query
parameter in your request. Pass it the name of the field to be ordered on.
Multiple fields can be specified with a comma-separated list. Examples:

- `create_time`
- `update_time asc`
- `create_time desc, update_time asc`

Fields supported in `order_by`:

| Field Name     |
| -------------- |
| `create_time` |
| `update_time` |

## Pagination

The reference implementation of Results API supports pagination for results, records
and logs. The default number of objects in a single page is 50 and the maximum number is 10000.

To paginate the response, include the `page_size` query parameter in your request.
It must be an integer value between 0 and, 10000. If the `page_size` is less than
the number of total objects available for the particular query, the response will
include a `NextPageToken` in the response. You can pass this value to `page_token`
query parameter to fetch the next page. Both the queries are independent and can
be used individually or together.

| Name         | Description                                     |
| ------------ | ----------------------------------------------- |
| `page_size`  | The number of objects to fetch in the response. |
| `page_token` | Token of the page to be fetched.                |

## Reading results across parents

Results can be read across parents by specifying `-` as the parent name. This is
useful for listing all results stored in the system without prior knowledge about
the available parents.

## Reading Records across Results

Records can be read across Results by specifying `-` as the Result name part, or
across parents by specifying `-` as the parent name.
(e.g. `default/results/-` or `-/results/-`). This can be used to read and filter
matching Records without knowing the exact Result name.

## Metrics

The API Server includes an HTTP server for exposing gRPC server Prometheus
metrics. By default, the Service exposes metrics on port `:9090`. For more
details on the structure of the metrics, see
<https://github.com/grpc-ecosystem/go-grpc-prometheus#metrics>.

## Health

The API Server includes gRPC and REST endpoints for monitoring the serving status
of the API server as well as serving status of individual services.

### Checking Status

```sh
# Check status of API server using gRPC
grpcurl --insecure localhost:8080 grpc.health.v1.Health/Check

# Check status of individual service using gRPC
grpcurl --insecure -d '{"service": "tekton.results.v1alpha2.Results"}' localhost:8080 grpc.health.v1.Health/Check

# Check status of API server using REST
curl -k https://localhost:8080/healthz

# Check status of individual service using REST
curl -k https://localhost:8080/healthz?service=tekton.results.v1alpha2.Results
```

## Profiling

The API Server includes an HTTP server for exposing golang's debug profiles. By default, the Service is disabled and exposes debug profiles on port `:6060`. For more
details on the using the profiles, see
<https://pkg.go.dev/net/http/pprof>.


## References

- [OpenAPI Specification](openapi.yaml)
