## tkn-results pipelinerun describe

Describe a PipelineRun

### Synopsis

Describe a PipelineRun by name or UID. If --uid is provided, then PipelineRun name is optional.

```
tkn-results pipelinerun describe [pipelinerun-name]
```

### Examples

```
Describe a PipelineRun in namespace 'foo':
    tkn-results pipelinerun describe my-pipelinerun -n foo

Describe a PipelineRun in 'default' namespace:
    tkn-results pipelinerun describe my-pipelinerun

```

### Options

```
  -A, --all-namespaces   use all namespaces
  -h, --help             help for describe
      --uid string       UID of the PipelineRun to describe the details
```

### Options inherited from parent commands

```
  -a, --addr string                Result API server address. If not specified, tkn-result would port-forward to service/tekton-results-api-service automatically
      --api-path string            api path to use (default: value provided in config set command)
  -t, --authtoken string           authorization bearer token to use for authenticated requests
  -c, --context string             name of the kubeconfig context to use (default: kubectl config current-context)
      --host string                host to use (default: value provided in config set command)
      --insecure                   determines whether to run insecure GRPC tls request
      --insecure-skip-tls-verify   skip server's certificate validation for requests (default: false)
  -k, --kubeconfig string          kubectl config file (default: $HOME/.kube/config)
  -n, --namespace string           namespace to use (default: from $KUBECONFIG)
      --portforward                enable auto portforwarding to tekton-results-api-service, when addr is set and portforward is true, tkn-results will portforward tekton-results-api-service automatically (default true)
      --sa string                  ServiceAccount to use instead of token for authorization and authentication
      --sa-ns string               ServiceAccount Namespace, if not given, it will be taken from current context
      --token string               bearer token to use (default: value provided in config set command)
      --v1alpha2                   use v1alpha2 API for get log command
```

### SEE ALSO

* [tkn-results pipelinerun](tkn-results_pipelinerun.md)	 - Query PipelineRuns

