# SimpleUI

This folder contains a test HTTP server for rendering Results in a very basic
UI.

This is **not** intended to be a replacement UI for
[dashboard](`https://github.com/tektoncd/dashboard) - this is only here for
local testing to help visualize result data.

## How to run

```sh
# Port forward API Server to make it available locally.
# This should be ran in its own shell or backgrounded, since the command blocks.
$ kubectl port-forward -n tekton-pipelines service/tekton-results-api-service 50051:50051
# Start the HTTP server
$ go run .
```
