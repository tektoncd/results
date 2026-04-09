## tkn-results taskrun logs

Get logs for a TaskRun

### Synopsis

Get logs for a TaskRun by name or UID. If --uid is provided, the TaskRun name is optional.

NOTE:
Logs are not supported for the system namespace or for the default namespace used by LokiStack.
Logs are only available for completed TaskRuns. Running TaskRuns do not have logs available yet.

```
tkn-results taskrun logs [taskrun-name]
```

### Examples

```
Get logs for a TaskRun named 'foo' in the current namespace:
  tkn-results taskrun logs foo

Get logs for a TaskRun in a specific namespace:
  tkn-results taskrun logs foo -n my-namespace

Get logs for a TaskRun by UID if there are multiple TaskRun with the same name:
  tkn-results taskrun logs --uid 12345678-1234-1234-1234-1234567890ab

```

### Options

```
  -h, --help         help for logs
      --uid string   UID of the TaskRun to get logs for
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

