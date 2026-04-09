## tkn-results taskrun describe

Describe a TaskRun

### Synopsis

Describe a TaskRun by name or UID. If --uid is provided, then TaskRun name is optional.

```
tkn-results taskrun describe [taskrun-name]
```

### Examples

```
Describe a TaskRun in namespace 'foo':
    tkn-results taskrun describe my-taskrun -n foo

Describe a TaskRun in the current namespace
    tkn-results taskrun describe my-taskrun

Describe a TaskRun as yaml
    tkn-results taskrun describe my-taskrun -o yaml

Describe a TaskRun as json
    tkn-results taskrun describe my-taskrun -o json

```

### Options

```
  -h, --help            help for describe
  -o, --output string   Output format. One of: json|yaml (Default format is used if not specified)
      --uid string      UID of the TaskRun to describe
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

