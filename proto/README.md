## Regenerating proto libraries

1. Verify [protoc + plugins are installed](https://grpc.io/docs/languages/go/quickstart/).
2. Rebuild the generated Go code

```sh
$ go generate -v ./...
```
