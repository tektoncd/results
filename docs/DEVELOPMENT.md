# Developing

## Tooling

The following tools are used by the project:

- [git](https://git-scm.com/doc)
- [go](https://golang.org/doc/install)
- [kind](https://kind.sigs.k8s.io)
- [ko](https://github.com/google/ko)
- [kubectl](https://kubernetes.io/docs/reference/kubectl/overview/)
- [openssl](https://www.openssl.org/)
- [protoc with gRPC Go plugins](https://grpc.io/docs/languages/go/quickstart/)

### Recommended Tooling

These tools are recommended, but not required:

- [prettier](https://prettier.io/)
- [sqlite3](https://www.sqlite.org/index.html)

## Quickstart

The easiest way to get started is to use the e2e testing scripts to bootstrap
installation. These are configured to install real versions of Tekton Pipelines
/ Results, using [kind](https://kind.sigs.k8s.io) by default.

```sh
export KO_DOCKER_REPO="kind.local"
$ ./test/e2e/00-setup.sh    # sets up kind cluster
$ ./test/e2e/01-install.sh  # installs pipelines, configures db, installs results api/watcher.
```

`01-install.sh` uses the default kubectl context, so this can be ran on both
kind or real Kubernetes clusters. See [test/e2e/README.md](/test/e2e/README.md)
for configurable options for these scripts.

### Deploying individual components

You can redeploy individual components via ko. Just make sure to
[configure ko with kind](https://github.com/google/ko/blob/main/README.md#with-kind).

```sh
$ export KO_DOCKER_REPO=kind.local
$ ko apply -f config/watcher.yaml
```

### Re-deploying all Results components

You can redeploy all components with kubectl and ko. Just make sure to
[configure ko with kind](https://github.com/google/ko/blob/main/README.md#with-kind).

```sh
$ export KO_DOCKER_REPO=kind.local
$ kubectl kustomize ./config | ko apply -f -
```

## Debugging

The easiest way to make requests to the API Server for manual inspection is
using
[`kubectl port-forward`](https://kubernetes.io/docs/tasks/access-application-cluster/port-forward-access-application-cluster/) +
[`grpc_cli`](https://github.com/grpc/grpc/blob/master/doc/command_line_tool.md).

```sh
# Prepare a custom Service Account that will be used for debugging purposes
$ kubectl create sa tekton-results-debug -n tekton-pipelines

# Grant required privileges to the Service Account
$ kubectl create clusterrolebinding tekton-results-debug --clusterrole=tekton-results-readonly --serviceaccount=tekton-pipelines:tekton-results-debug

# Proxies the remote Service to localhost.
$ kubectl port-forward -n tekton-pipelines service/tekton-results-api-service 50051  # This will block, so run this in a separate shell or background the process.

# If using self-signed certs, download the API Server certificate locally and configure gRPC.
# (if using a cert that's already present in your system pool, this can be skipped)
$ kubectl get secrets tekton-results-tls -n tekton-pipelines --template='{{index .data "tls.crt"}}' | base64 -d > /tmp/results.crt
$ export GRPC_DEFAULT_SSL_ROOTS_FILE_PATH=/tmp/results.crt

# Lists the available gRPC services.
$ grpc_cli ls --channel_creds_type=ssl --ssl_target=tekton-results-api-service.tekton-pipelines.svc.cluster.local localhost:50051
grpc.reflection.v1alpha.ServerReflection
tekton.results.v1alpha2.Results

# Create gRPC access token
$ export GRPC_ACCESS_TOKEN=$(kubectl create token tekton-results-debug -n tekton-pipelines)
# Makes a request to the Results service.
$ grpc_cli call --channel_creds_type=ssl --ssl_target=tekton-results-api-service.tekton-pipelines.svc.cluster.local --call_creds=access_token=$GRPC_ACCESS_TOKEN localhost:50051 tekton.results.v1alpha2.Results.ListResults 'parent: "default"'
connecting to localhost:50051
results {
  name: "default/results/9b7714d0-cbd3-40c6-87ec-bcbd9f199985"
  id: "948c645f-692f-4e1c-8ac7-b5720d9a7951"
  created_time {
    seconds: 1610473994
  }
  etag: "948c645f-692f-4e1c-8ac7-b5720d9a7951-1610473994386247754"
  updated_time {
    seconds: 1610473994
  }
}
Rpc succeeded with OK status
```

NOTE: you can ignore `Unexpected service config health received` errors - this
is because we do not have health checking set up yet.

## Conventions

- Style Guides - [Go](https://github.com/golang/go/wiki/CodeReviewComments)
- [API Design](https://aip.dev)
- Formatting
  1. Language recommended tools first (e.g. gofmt)
  2. Default to `prettier` (recommended command:
     `prettier --write --prose-wrap always`)

## Testing

### Unit Tests

```sh
$ go test ./...
```

### E2E Tests

See [test/e2e/README.md](/test/e2e/README.md)

### Change log level

You can change log level controllers for development purpose. Edit configmap:

```sh
kubectl edit cm config-logging -n tekton-pipelines
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
