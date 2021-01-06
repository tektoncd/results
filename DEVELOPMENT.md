# Developing

## Tooling

The following tools are used by the project:

- [git](https://git-scm.com/doc)
- [go](https://golang.org/doc/install)
- [kind](https://kind.sigs.k8s.io)
- [ko](https://github.com/google/ko)
- [kubectl](https://github.com/google/ko)
- [openssl](https://www.openssl.org/)
- [protoc with gRPC Go plugins](https://grpc.io/docs/languages/go/quickstart/)

### Recommended Tooling

These tools are recommended, but not required:

- [prettier](https://prettier.io/)
- [sqlite3](https://www.sqlite.org/index.html)

## Quickstart

The easiest way to get started is to use the e2e testing scripts to bootstrap installation. These are configured to install real versions of Tekton Pipelines / Results, using [kind](https://kind.sigs.k8s.io) by default.

```sh
$ ./test/e2e/00-setup.sh    # sets up kind cluster
$ ./test/e2e/01-install.sh  # installs pipelines, configures db, installs results api/watcher.
```

`01-install.sh` uses the default kubectl context, so this can be ran on both kind or real Kubernetes clusters. See [test/e2e/README.md](test/e2e/README.md) for configurable options for these scripts.

### Deploying individual components

You can redeploy individual components via ko. Just make sure to use [configure ko with kind](https://github.com/google/ko/blob/master/README.md#with-kind).

```sh
$ export KO_DOCKER_REPO=kind.local
$ ko apply -f config/watcher.yaml
```

<!--- TODO: grpc_cli command instructions --->

## Conventions

- Style Guides - [Go](https://github.com/golang/go/wiki/CodeReviewComments)
- [API Design](https://aip.dev)
- Formatting
    1. Language recommended tools first (e.g. gofmt)
    2. Default to `prettier` (recommended command: `prettier --write --prose-wrap always`)

## Testing

### Unit Tests

```sh
$ go test ./...
```

### E2E Tests

See [test/e2e/README.md](test/e2e/README.md)

## Recommended Reading

- [pipeline/DEVELOPMENT.md](https://github.com/tektoncd/pipeline/blob/master/DEVELOPMENT.md)