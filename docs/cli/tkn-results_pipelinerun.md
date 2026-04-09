## tkn-results pipelinerun

Query PipelineRuns

### Synopsis

Query PipelineRuns stored in Tekton Results.

This command allows you to list PipelineRuns stored in Tekton Results.
You can filter results by namespace, labels and other criteria.

Examples:
  # List PipelineRuns in a namespace
  tkn-results pipelinerun list -n default

  # List PipelineRuns with a specific label
  tkn-results pipelinerun list -L app=myapp

  # List PipelineRuns from all namespaces
  tkn-results pipelinerun list -A

  # List PipelineRuns with limit
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

### SEE ALSO

* [tkn-results](tkn-results.md)	 - Tekton Results CLI
* [tkn-results pipelinerun describe](tkn-results_pipelinerun_describe.md)	 - Describe a PipelineRun
* [tkn-results pipelinerun list](tkn-results_pipelinerun_list.md)	 - List PipelineRuns in a namespace
* [tkn-results pipelinerun logs](tkn-results_pipelinerun_logs.md)	 - Get logs for a PipelineRun

