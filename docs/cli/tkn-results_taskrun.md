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

### SEE ALSO

* [tkn-results](tkn-results.md)	 - Tekton Results CLI
* [tkn-results taskrun describe](tkn-results_taskrun_describe.md)	 - Describe a TaskRun
* [tkn-results taskrun list](tkn-results_taskrun_list.md)	 - List TaskRuns in a namespace
* [tkn-results taskrun logs](tkn-results_taskrun_logs.md)	 - Get logs for a TaskRun

