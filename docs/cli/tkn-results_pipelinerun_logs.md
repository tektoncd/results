## tkn-results pipelinerun logs

Get logs for a PipelineRun

### Synopsis

Get logs for a PipelineRun by name or UID. If --uid is provided, the PipelineRun name is optional.

NOTE:
Logs are not supported for the system namespace or for the default namespace used by LokiStack.
Additionally, PipelineRun logs are not supported for S3 log storage.

```
tkn-results pipelinerun logs [pipelinerun-name]
```

### Examples

```
Get logs for a PipelineRun named 'foo' in the current namespace:
  tkn-results pipelinerun logs foo

Get logs for a PipelineRun in a specific namespace:
  tkn-results pipelinerun logs foo -n my-namespace

Get logs for a PipelineRun by UID if there are multiple PipelineRuns with the same name:
  tkn-results pipelinerun logs --uid 12345678-1234-1234-1234-1234567890ab

```

### Options

```
  -h, --help         help for logs
      --uid string   UID of the PipelineRun to get logs for
```

### Options inherited from parent commands

```
  -a, --addr string                [To be deprecated] Result API server address. If not specified, tkn-result would port-forward to service/tekton-results-api-service automatically
      --api-path string            api path to use (default: value provided in config set command)
  -t, --authtoken string           [To be deprecated] authorization bearer token to use for authenticated requests
  -c, --context string             name of the kubeconfig context to use (default: kubectl config current-context)
      --host string                host to use (default: value provided in config set command)
      --insecure                   [To be deprecated] determines whether to run insecure GRPC tls request
      --insecure-skip-tls-verify   skip server's certificate validation for requests (default: false)
  -k, --kubeconfig string          kubectl config file (default: $HOME/.kube/config)
  -n, --namespace string           namespace to use (default: from $KUBECONFIG)
      --portforward                [To be deprecated] enable auto portforwarding to tekton-results-api-service, when addr is set and portforward is true, tkn-results will portforward tekton-results-api-service automatically (default true)
      --sa string                  [To be deprecated] ServiceAccount to use instead of token for authorization and authentication
      --sa-ns string               [To be deprecated] ServiceAccount Namespace, if not given, it will be taken from current context
      --token string               bearer token to use (default: value provided in config set command)
      --v1alpha2                   [To be deprecated] use v1alpha2 API for get log command
```

### SEE ALSO

* [tkn-results pipelinerun](tkn-results_pipelinerun.md)	 - Query PipelineRuns

