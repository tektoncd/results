# Developing

## Tooling

The following tools are used by the project:

- [git](https://git-scm.com/doc)
- [go](https://golang.org/doc/install) **>=go1.19**
- [kind](https://kind.sigs.k8s.io)
- [ko](https://github.com/google/ko)
- [kubectl](https://kubernetes.io/docs/reference/kubectl/overview/)
- [openssl](https://www.openssl.org/)
- [protoc with gRPC Go plugins](https://grpc.io/docs/languages/go/quickstart/)

### Recommended Tooling

These tools are recommended, but not required:

- [prettier](https://prettier.io/)
- [sqlite3](https://www.sqlite.org/index.html)
- [grpc_cli](https://github.com/grpc/grpc/blob/master/doc/command_line_tool.md)
- [grpcurl](https://github.com/fullstorydev/grpcurl)
- [curl](https://curl.se/download.html)

## Quickstart

The easiest way to get started is to use the e2e testing scripts to bootstrap
installation. These are configured to install real versions of Tekton Pipelines
/ Results, using [kind](https://kind.sigs.k8s.io) by default.

```sh
export KO_DOCKER_REPO="kind.local"
./test/e2e/00-setup.sh    # sets up kind cluster
./test/e2e/01-install.sh  # installs pipelines, configures db, installs results api/watcher.
```

`01-install.sh` uses the default kubectl context, so this can be ran on both
kind or real Kubernetes clusters. See [test/e2e/README.md](/test/e2e/README.md)
for configurable options for these scripts.

### Deploying individual components

You can redeploy individual components via ko. Just make sure to
[configure ko with kind](https://github.com/google/ko/blob/main/README.md#with-kind).

```sh
export KO_DOCKER_REPO=kind.local
ko apply -f config/watcher.yaml
```

### Re-deploying all Results components

You can redeploy all components with kubectl and ko. Just make sure to
[configure ko with kind](https://github.com/google/ko/blob/main/README.md#with-kind).

```sh
export KO_DOCKER_REPO=kind.local
kubectl kustomize ./test/e2e/kustomize/ | ko apply -f -
```

## Debugging

The easiest way to make requests to the API Server for manual inspection is
using
[`kubectl port-forward`](https://kubernetes.io/docs/tasks/access-application-cluster/port-forward-access-application-cluster/) +
[`grpc_cli`](https://github.com/grpc/grpc/blob/master/doc/command_line_tool.md) or [`curl`](https://curl.se/download.html).

- Prepare a custom Service Account that will be used for debugging purposes

```sh
kubectl create sa tekton-results-debug -n tekton-pipelines
```

- Grant required privileges to the Service Account

```sh
kubectl create clusterrolebinding tekton-results-debug --clusterrole=tekton-results-readonly --serviceaccount=tekton-pipelines:tekton-results-debug
```

- Create access token

```sh
export ACCESS_TOKEN=$(kubectl create token tekton-results-debug -n tekton-pipelines)
```

### Using `grpc_cli` + `kubectl port-forward`

- Proxies the remote Service to localhost for gRPC. This will block, so run this in a separate shell or background the process.

```sh
kubectl port-forward -n tekton-pipelines service/tekton-results-api-service 8080
```

- If using self-signed certs, download the API Server certificate locally and configure gRPC. (if using a cert that's already present in your system pool, this can be skipped)

```sh
kubectl get secrets tekton-results-tls -n tekton-pipelines --template='{{index .data "tls.crt"}}' | base64 -d > /tmp/results.crt
export GRPC_DEFAULT_SSL_ROOTS_FILE_PATH=/tmp/results.crt
```

- List the available gRPC services

```sh
grpc_cli ls --channel_creds_type=ssl --ssl_target=tekton-results-api-service.tekton-pipelines.svc.cluster.local localhost:8080
```

```sh
# output: available gRPC services
grpc.health.v1.Health
grpc.reflection.v1alpha.ServerReflection
tekton.results.v1alpha2.Logs
tekton.results.v1alpha2.Results
```

- Makes a request to the Results service

```sh
grpc_cli call --channel_creds_type=ssl --ssl_target=tekton-results-api-service.tekton-pipelines.svc.cluster.local --call_creds=access_token=$ACCESS_TOKEN localhost:8080 tekton.results.v1alpha2.Results.ListResults 'parent: "default"'
```

```sh
# output: list of results
connecting to localhost:8080
results {
  name: "default/results/7afa9067-5001-4d93-b715-49854a770412"
  id: "b74a3317-e6c0-421c-85d9-54b0f3d4b4c6"
  created_time {
    seconds: 1677742028
    nanos: 143729000
  }
  etag: "b74a3317-e6c0-421c-85d9-54b0f3d4b4c6-1677742039224211588"
  updated_time {
    seconds: 1677742039
    nanos: 224211000
  }
  uid: "b74a3317-e6c0-421c-85d9-54b0f3d4b4c6"
  create_time {
    seconds: 1677742028
    nanos: 143729000
  }
  update_time {
    seconds: 1677742039
    nanos: 224211000
  }
  summary {
    record: "default/results/7afa9067-5001-4d93-b715-49854a770412/records/7afa9067-5001-4d93-b715-49854a770412"
    type: "tekton.dev/v1beta1.TaskRun"
    end_time {
      seconds: 1677742039
    }
    status: SUCCESS
  }
}
results {
  name: "default/results/c360def0-d77e-4a3f-a1b0-5b0753e7d5af"
  id: "9514f318-9329-485b-871c-77a4a6904891"
  created_time {
    seconds: 1677742085
    nanos: 535047000
  }
  etag: "9514f318-9329-485b-871c-77a4a6904891-1677742090308632274"
  updated_time {
    seconds: 1677742090
    nanos: 308632000
  }
  uid: "9514f318-9329-485b-871c-77a4a6904891"
  create_time {
    seconds: 1677742085
    nanos: 535047000
  }
  update_time {
    seconds: 1677742090
    nanos: 308632000
  }
  summary {
    record: "default/results/c360def0-d77e-4a3f-a1b0-5b0753e7d5af/records/c360def0-d77e-4a3f-a1b0-5b0753e7d5af"
    type: "tekton.dev/v1beta1.TaskRun"
    end_time {
      seconds: 1677742090
    }
    status: SUCCESS
  }
}
Rpc succeeded with OK status
```

NOTE: you can ignore `Unexpected service config health received` errors: this
is because we do not have health checking set up yet. Please refer <https://github.com/tektoncd/results/issues/414>.

### Using `curl` + `kubectl port-forward`

See the available REST endpoints in the [OpenAPI specification](api/rest-api-spec.md) docs. The API request URL is of the format `https://{server_url}/apis/results.tekton.dev/v1alpha2/parents/{name/path-to-the-resource}`. For debugging server_url is `localhost:port-exposed`.

- Proxy the remote Service to localhost for REST. This will block, so run this in a separate shell or background the process.

```sh
kubectl port-forward -n tekton-pipelines service/tekton-results-api-service 8080
```

- Make a curl request to the Results REST API.

```sh
curl --insecure \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Accept: application/json" \
  https://localhost:8080/apis/results.tekton.dev/v1alpha2/parents/default/results
```

This particular request lists the results under default namespace.

```json
{
  "results": [
    {
      "name": "default/results/640d1af3-9c75-4167-8167-4d8e4f39d403",
      "id": "338481c9-3bc6-472f-9d1b-0f7705e6cb8c",
      "uid": "338481c9-3bc6-472f-9d1b-0f7705e6cb8c",
      "createdTime": "2023-03-02T07:26:48.972907Z",
      "createTime": "2023-03-02T07:26:48.972907Z",
      "updatedTime": "2023-03-02T07:26:54.191114Z",
      "updateTime": "2023-03-02T07:26:54.191114Z",
      "annotations": {},
      "etag": "338481c9-3bc6-472f-9d1b-0f7705e6cb8c-1677742014191114634",
      "summary": {
        "record": "default/results/640d1af3-9c75-4167-8167-4d8e4f39d403/records/640d1af3-9c75-4167-8167-4d8e4f39d403",
        "type": "tekton.dev/v1beta1.TaskRun",
        "startTime": null,
        "endTime": "2023-03-02T07:26:54Z",
        "status": "SUCCESS",
        "annotations": {}
      }
    },
    {
      "name": "default/results/c360def0-d77e-4a3f-a1b0-5b0753e7d5af",
      "id": "9514f318-9329-485b-871c-77a4a6904891",
      "uid": "9514f318-9329-485b-871c-77a4a6904891",
      "createdTime": "2023-03-02T07:28:05.535047Z",
      "createTime": "2023-03-02T07:28:05.535047Z",
      "updatedTime": "2023-03-02T07:28:10.308632Z",
      "updateTime": "2023-03-02T07:28:10.308632Z",
      "annotations": {},
      "etag": "9514f318-9329-485b-871c-77a4a6904891-1677742090308632274",
      "summary": {
        "record": "default/results/c360def0-d77e-4a3f-a1b0-5b0753e7d5af/records/c360def0-d77e-4a3f-a1b0-5b0753e7d5af",
        "type": "tekton.dev/v1beta1.TaskRun",
        "startTime": null,
        "endTime": "2023-03-02T07:28:10Z",
        "status": "SUCCESS",
        "annotations": {}
      }
    }
  ],
  "nextPageToken": ""
}
```

## Conventions

- Style Guides: [Go](https://github.com/golang/go/wiki/CodeReviewComments)
- [API Design](https://aip.dev)
- Formatting
  1. Language recommended tools first (e.g. gofmt)
  2. Default to `prettier` (recommended command:
     `prettier --write --prose-wrap always`)

## Testing

### Unit Tests

```sh
go test ./...
```

### E2E Tests

See [test/e2e/README.md](/test/e2e/README.md)

### Change log level

You can change log level controllers for development purpose. Edit configmap:

```sh
kubectl edit cm tekton-results-config-logging -n tekton-pipelines
```

Change "zap-logger-config" field "level" and save it.

```yaml
apiVersion: v1
data:
  loglevel.controller: info
  loglevel.webhook: info
  zap-logger-config: |
    {
      "level": "info",
  ...
```

Log levels supported by Zap logger are:

debug - fine-grained debugging
info - normal logging
warn - unexpected but non-critical errors
error - critical errors; unexpected during normal operation
dpanic - in debug mode, trigger a panic (crash)
panic - trigger a panic (crash)
fatal - immediately exit with exit status 1 (failure)

## Recommended Reading

- [pipeline/DEVELOPMENT.md](https://github.com/tektoncd/pipeline/blob/main/DEVELOPMENT.md)
