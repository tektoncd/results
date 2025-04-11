## tkn-results pipelinerun list

List PipelineRuns in a namespace

```
tkn-results pipelinerun list
```

### Examples

```
List all PipelineRuns in a namespace 'foo':
    tkn-results pipelinerun list -n foo

List all PipelineRuns in 'default' namespace:
    tkn-results pipelinerun list -n default

```

### Options

```
  -h, --help               help for list
  -l, --limit int32        Limit the number of PipelineRuns to return
  -n, --namespace string   Namespace to list PipelineRuns in (default "default")
```

### Options inherited from parent commands

```
  -a, --addr string        Result API server address. If not specified, tkn-result would port-forward to service/tekton-results-api-service automatically
  -t, --authtoken string   authorization bearer token to use for authenticated requests
      --insecure           determines whether to run insecure GRPC tls request
      --portforward        enable auto portforwarding to tekton-results-api-service, when addr is set and portforward is true, tkn-results will portforward tekton-results-api-service automatically (default true)
      --sa string          ServiceAccount to use instead of token for authorization and authentication
      --sa-ns string       ServiceAccount Namespace, if not given, it will be taken from current context
      --v1alpha2           use v1alpha2 API for get log command
```

### SEE ALSO

* [tkn-results pipelinerun](tkn-results_pipelinerun.md)	 - Query PipelineRuns

