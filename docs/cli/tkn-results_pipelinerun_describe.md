## tkn-results pipelinerun describe

Describe a PipelineRun

### Synopsis

Describe a PipelineRun by name or UID. If --uid is provided, then PipelineRun name is optional.

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
  -a, --addr string                [To be deprecated] Result API server address. If not specified, tkn-result would port-forward to service/tekton-results-api-service automatically
      --api-path string            api path to use (default: value provided in config set command)
  -t, --authtoken string           [To be deprecated] authorization bearer token to use for authenticated requests
  -c, --context string             name of the kubeconfig context to use (default: kubectl config current-context)
      --host string                host to use (default: value provided in config set command)
      --insecure                   [To be deprecated] determines whether to run insecure GRPC tls request
      --insecure-skip-tls-verify   skip server's certificate validation for requests (default: false)
  -k, --kubeconfig string          kubectl config file (default: $HOME/.kube/config)
  -n, --namespace string           namespace to use (default: from $KUBECONFIG)
      --portforward                [To be deprecated] enable auto portforwarding to tekton-results-api-service, when addr is set and portforward is true, tkn-results will portforward tekton-results-api-service automatically (default true)
      --sa string                  [To be deprecated] ServiceAccount to use instead of token for authorization and authentication
      --sa-ns string               [To be deprecated] ServiceAccount Namespace, if not given, it will be taken from current context
      --token string               bearer token to use (default: value provided in config set command)
      --v1alpha2                   [To be deprecated] use v1alpha2 API for get log command
```

### SEE ALSO

* [tkn-results pipelinerun](tkn-results_pipelinerun.md)	 - Query PipelineRuns

