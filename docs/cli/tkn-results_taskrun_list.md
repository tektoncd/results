## tkn-results taskrun list

List TaskRuns in a namespace

```
tkn-results taskrun list [task-name]
```

### Examples

```
List all TaskRuns in a namespace 'foo':
    tkn-results taskrun list -n foo

List TaskRuns with a specific label:
    tkn-results taskrun list -L app=myapp

List TaskRuns with multiple labels:
    tkn-results taskrun list --label app=myapp,env=prod

List TaskRuns from all namespaces:
    tkn-results taskrun list -A

List TaskRuns with limit of 20 per page:
    tkn-results taskrun list --limit=20

List TaskRuns for a specific task:
    tkn-results taskrun list foo -n namespace

List TaskRuns with partial task name match:
    tkn-results taskrun list build -n namespace

List TaskRuns for a specific PipelineRun:
    tkn-results taskrun list --pipelinerun my-pipeline-run -n namespace

```

### Options

```
  -A, --all-namespaces       List TaskRuns from all namespaces
  -h, --help                 help for list
  -L, --label string         Filter by label (format: key=value[,key=value...])
      --limit int32          Maximum number of TaskRuns to return (must be between 5 and 1000, default is 50) (default 50)
      --pipelinerun string   Filter TaskRuns by PipelineRun name. Note that multiple PipelineRuns can have the same name, so this will return TaskRuns from all PipelineRuns with the matching name.
      --single-page          Return only a single page of results (default true)
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

* [tkn-results taskrun](tkn-results_taskrun.md)	 - Query TaskRuns

