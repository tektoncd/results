## tkn-results pipelinerun

Query PipelineRuns

### Synopsis

Query PipelineRuns stored in Tekton Results.

This command allows you to list PipelineRuns stored in Tekton Results.
You can filter results by namespace, labels, and other criteria.

Examples:
  # List PipelineRuns in a namespace
  tkn-results pipelinerun list -n default

  # List PipelineRuns with a specific label
  tkn-results pipelinerun list -l app=myapp

  # List PipelineRuns from all namespaces
  tkn-results pipelinerun list -A

  # List PipelineRuns with pagination
  tkn-results pipelinerun list --limit=20

### Options

```
      --api-path string            api path to use (default: value provided in config set command)
  -c, --context string             name of the kubeconfig context to use (default: kubectl config current-context)
  -h, --help                       help for pipelinerun
      --host string                host to use (default: value provided in config set command)
      --insecure-skip-tls-verify   skip server's certificate validation for requests (default: false)
  -k, --kubeconfig string          kubectl config file (default: $HOME/.kube/config)
  -n, --namespace string           namespace to use (default: from $KUBECONFIG)
      --token string               bearer token to use (default: value provided in config set command)
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

* [tkn-results](tkn-results.md)	 - Tekton Results CLI
* [tkn-results pipelinerun list](tkn-results_pipelinerun_list.md)	 - List PipelineRuns in a namespace

