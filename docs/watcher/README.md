# Result Watcher

The Result Watcher is a Kubernetes Controller that watches for changes to certain Tekton types and automatically creates/updates their data in the Result API.

## Supported Types

The Watcher currently supports the following types:

- `tekton.dev/v1beta1 TaskRun`
- `tekton.dev/v1beta1 PipelineRun`

## Result Grouping

The Watcher uses annotations to automatically detect and group related Records into the same Result. The following annotations are recognized (in order of precedence):

- `results.tekton.dev/result`
- `triggers.tekton.dev/triggers-eventid`
- `tekton.dev/pipelineRun`

If no annotation is detected, the Watcher will automatically generate a new Result name for the object.