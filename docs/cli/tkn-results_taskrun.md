## tkn-results taskrun

Query TaskRuns

### Synopsis

Query TaskRuns stored in Tekton Results.

This command allows you to list TaskRuns stored in Tekton Results.
You can filter results by namespace, labels and other criteria.

Examples:
  # List TaskRuns in a namespace
  tkn-results taskrun list -n default

  # List TaskRuns with a specific label
  tkn-results taskrun list -L app=myapp

  # List TaskRuns from all namespaces
  tkn-results taskrun list -A

  # List TaskRuns with limit
  tkn-results taskrun list --limit=20

### Options

```
      --api-path string            api path to use (default: value provided in config set command)
  -c, --context string             name of the kubeconfig context to use (default: kubectl config current-context)
  -h, --help                       help for taskrun
      --host string                host to use (default: value provided in config set command)
      --insecure-skip-tls-verify   skip server's certificate validation for requests (default: false)
  -k, --kubeconfig string          kubectl config file (default: $HOME/.kube/config)
  -n, --namespace string           namespace to use (default: from $KUBECONFIG)
      --token string               bearer token to use (default: value provided in config set command)
```

### Options inherited from parent commands

```
  -a, --addr string        [To be deprecated] Result API server address. If not specified, tkn-result would port-forward to service/tekton-results-api-service automatically
  -t, --authtoken string   [To be deprecated] authorization bearer token to use for authenticated requests
      --insecure           [To be deprecated] determines whether to run insecure GRPC tls request
      --portforward        [To be deprecated] enable auto portforwarding to tekton-results-api-service, when addr is set and portforward is true, tkn-results will portforward tekton-results-api-service automatically (default true)
      --sa string          [To be deprecated] ServiceAccount to use instead of token for authorization and authentication
      --sa-ns string       [To be deprecated] ServiceAccount Namespace, if not given, it will be taken from current context
      --v1alpha2           [To be deprecated] use v1alpha2 API for get log command
```

### SEE ALSO

* [tkn-results](tkn-results.md)	 - Tekton Results CLI
* [tkn-results taskrun describe](tkn-results_taskrun_describe.md)	 - Describe a TaskRun
* [tkn-results taskrun list](tkn-results_taskrun_list.md)	 - List TaskRuns in a namespace
* [tkn-results taskrun logs](tkn-results_taskrun_logs.md)	 - Get logs for a TaskRun

