## tkn-results config set

Set Tekton Results CLI configuration values

### Synopsis

Configure how the CLI connects to the Tekton Results API server.

Usage Modes:
1. Interactive: Prompts for values with defaults where available
   `tkn-results config set`

2. Manual: Specify values via flags
   `tkn-results config set --host=<url> --token=<token>`

Configuration Options:
- Host: Tekton Results API server URL
- Token: Bearer token (defaults to current kubeconfig token)
- API Path: API endpoint path
- TLS Settings: Certificate verification options

Use manual configuration when:
- Route is not in openshift-pipelines namespace
- Route name differs from tekton-results-api-service
- Using custom domain patterns
- On Kubernetes clusters (ingress hostnames vary)

Route Requirements (OpenShift):
- Route name: tekton-results-api-service
- Namespace: openshift-pipelines
- Expected URL format: `https://<route-name>-<namespace>.apps.<cluster-domain>`

If your route deviates from this standard format, use manual configuration.

```
tkn-results config set
```

### Examples

```
Configure with automatic detection and interactive prompts:
  tkn-results config set

Configure with specific parameters (no prompts):
  tkn-results config set --host=http://localhost:8080 --token=my-token

Configure with custom API path (no prompts):
  tkn-results config set --api-path=/api/v1

Configure with custom kubeconfig and context:
  tkn-results config set --kubeconfig=/path/to/kubeconfig --context=my-cluster
```

### Options

```
      --api-path string            api path to use (default: value provided in config set command)
  -c, --context string             name of the kubeconfig context to use (default: kubectl config current-context)
  -h, --help                       help for set
      --host string                host to use (default: value provided in config set command)
      --insecure-skip-tls-verify   skip server's certificate validation for requests (default: false)
  -k, --kubeconfig string          kubectl config file (default: $HOME/.kube/config)
  -n, --namespace string           namespace to use (default: from $KUBECONFIG)
      --token string               bearer token to use (default: value provided in config set command)
```

### Options inherited from parent commands

```
  -a, --addr string        [To be deprecated] Result API server address. If not specified, tkn-result would port-forward to service/tekton-results-api-service automatically
  -t, --authtoken string   [To be deprecated] authorization bearer token to use for authenticated requests
      --insecure           [To be deprecated] determines whether to run insecure GRPC tls request
      --portforward        [To be deprecated] enable auto portforwarding to tekton-results-api-service, when addr is set and portforward is true, tkn-results will portforward tekton-results-api-service automatically (default true)
      --sa string          [To be deprecated] ServiceAccount to use instead of token for authorization and authentication
      --sa-ns string       [To be deprecated] ServiceAccount Namespace, if not given, it will be taken from current context
      --v1alpha2           [To be deprecated] use v1alpha2 API for get log command
```

### SEE ALSO

* [tkn-results config](tkn-results_config.md)	 - Manage Tekton Results CLI configuration

