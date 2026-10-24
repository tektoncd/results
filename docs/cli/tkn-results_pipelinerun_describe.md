## tkn-results pipelinerun describe

Describe a PipelineRun

### Synopsis

Describe a PipelineRun by name or UID. If --uid is provided, then PipelineRun name is optional.

If multiple PipelineRuns match the given name, the most recent one is returned.
Use --uid to target a specific PipelineRun when needed.

```
tkn-results pipelinerun describe [pipelinerun-name]
```

### Examples

```
Describe a PipelineRun in namespace 'foo':
    tkn-results pipelinerun describe my-pipelinerun -n foo

Describe a PipelineRun in the current namespace:
    tkn-results pipelinerun describe my-pipelinerun

Describe a PipelineRun as yaml:
    tkn-results pipelinerun describe my-pipelinerun -o yaml

Describe a PipelineRun as json:
    tkn-results pipelinerun describe my-pipelinerun -o json

```

### Options

```
  -h, --help            help for describe
  -o, --output string   Output format. One of: json|yaml (Default format is used if not specified)
      --uid string      UID of the PipelineRun to describe
```

### Options inherited from parent commands

```
      --api-path string            api path to use (default: value provided in config set command)
      --as string                  username to impersonate for the operation (user or service account)
      --as-group stringArray       group to impersonate for the operation, this flag can be repeated to specify multiple groups
      --as-uid string              UID to impersonate for the operation
  -c, --context string             name of the kubeconfig context to use (default: kubectl config current-context)
      --host string                host to use (default: value provided in config set command)
      --insecure-skip-tls-verify   skip server's certificate validation for requests (default: false)
  -k, --kubeconfig string          kubectl config file (default: $KUBECONFIG, then $HOME/.kube/config)
  -n, --namespace string           namespace to use (default: the current kubeconfig context's namespace)
      --token string               bearer token to use (default: value provided in config set command)
```

### SEE ALSO

* [tkn-results pipelinerun](tkn-results_pipelinerun.md)	 - Query PipelineRuns

