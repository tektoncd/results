## tkn-results pipelinerun list

List PipelineRuns in a namespace

```
tkn-results pipelinerun list [pipeline-name]
```

### Examples

```
List all PipelineRuns in a namespace 'foo':
    tkn-results pipelinerun list -n foo

List all PipelineRuns in 'default' namespace:
    tkn-results pipelinerun list -n default

List all PipelineRuns using the pagination, not the single page
    tkn-results pipelinerun list --single-page false

List PipelineRuns with a specific label:
    tkn-results pipelinerun list -l app=myapp

List PipelineRuns with multiple label selectors:
    tkn-results pipelinerun list -l app=myapp,env=prod

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
  -l, --limit int32      Maximum number of PipelineRuns to return (must be between 5 and 1000 and defaults to 50) (default 50)
      --single-page      Return only a single page of results (default true)
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

