## tkn-results pipelinerun list

List PipelineRuns in a namespace

```
tkn-results pipelinerun list [pipeline-name]
```

### Examples

```
List all PipelineRuns in a namespace 'foo':
    tkn-results pipelinerun list -n foo

List PipelineRuns with a specific label:
    tkn-results pipelinerun list -L app=myapp

List PipelineRuns with multiple label selectors:
    tkn-results pipelinerun list -L app=myapp,env=prod

List PipelineRuns from all namespaces:
    tkn-results pipelinerun list -A

List PipelineRuns with limit of 20 per page:
    tkn-results pipelinerun list --limit=20

List PipelineRuns for a specific pipeline:
    tkn-results pipelinerun list foo -n namespace

List PipelineRuns with partial pipeline name match:
    tkn-results pipelinerun list build -n namespace

```

### Options

```
  -A, --all-namespaces   List PipelineRuns from all namespaces
  -h, --help             help for list
  -L, --label string     Filter by label (format: key=value,key2=value2)
      --limit int32      Maximum number of PipelineRuns to return (must be between 5 and 1000 and defaults to 50) (default 50)
      --single-page      Return only a single page of results (default true)
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

