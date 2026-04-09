## tkn-results pipelinerun logs

Get logs for a PipelineRun

### Synopsis

Get logs for a PipelineRun by name or UID. If --uid is provided, the PipelineRun name is optional.

NOTE:
Logs are not supported for the system namespace or for the default namespace used by LokiStack.
Additionally, PipelineRun logs are not supported for S3 log storage.
Logs are only available for completed PipelineRuns. Running PipelineRuns do not have logs available yet.

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
      --api-path string            api path to use (default: value provided in config set command)
  -c, --context string             name of the kubeconfig context to use (default: kubectl config current-context)
      --host string                host to use (default: value provided in config set command)
      --insecure-skip-tls-verify   skip server's certificate validation for requests (default: false)
  -k, --kubeconfig string          kubectl config file (default: $HOME/.kube/config)
  -n, --namespace string           namespace to use (default: from $KUBECONFIG)
      --token string               bearer token to use (default: value provided in config set command)
```

### SEE ALSO

* [tkn-results pipelinerun](tkn-results_pipelinerun.md)	 - Query PipelineRuns

