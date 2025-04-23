## tkn-results taskrun logs

Get logs for a TaskRun

### Synopsis

Get logs for a TaskRun by name or UID. If --uid is provided, the TaskRun name is optional.

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

Get logs for a TaskRun from all namespaces:
  tkn-results taskrun logs foo -A

```

### Options

```
  -A, --all-namespaces             use all namespaces
      --api-path string            api path to use (default: value provided in config set command)
  -c, --context string             name of the kubeconfig context to use (default: kubectl config current-context)
  -h, --help                       help for logs
      --host string                host to use (default: value provided in config set command)
      --insecure-skip-tls-verify   skip server's certificate validation for requests (default: false)
  -k, --kubeconfig string          kubectl config file (default: $HOME/.kube/config)
  -n, --namespace string           namespace to use (default: from $KUBECONFIG)
      --token string               bearer token to use (default: value provided in config set command)
      --uid string                 UID of the TaskRun to get logs for
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

* [tkn-results taskrun](tkn-results_taskrun.md)	 - Query TaskRuns

