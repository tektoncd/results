## tkn-results config set

Configure Tekton Results CLI settings

### Synopsis

Configure settings for the Tekton Results CLI.

This command allows you to configure how the CLI interacts with the Tekton Results API server.
It can automatically detect the API server in your cluster or allow manual configuration.

The command will:
1. Automatically detect the Tekton Results API server in your cluster
2. Prompt for any missing configuration values
3. Save the configuration for future use

Automatic Detection:
- Cluster context and namespace
- API server endpoint
- Service account token (if available)

Manual Configuration (if automatic detection fails):
- API server host (e.g., http://localhost:8080)
- Authentication token
- Additional cluster settings

Configuration Options:
  --host                    API server host URL
  --token                   Authentication token
  --api-path                API server path prefix
  --insecure-skip-tls-verify Skip TLS certificate verification
  --kubeconfig, -k          Path to kubeconfig file
  --context, -c             Kubernetes context to use
  --namespace, -n           Kubernetes namespace

Note: Interactive prompts will be skipped if any configuration flag (host, token, api-path, insecure-skip-tls-verify) is used.

Examples:
  # Configure with automatic detection and interactive prompts
  tkn-results config set

  # Configure with specific parameters (no prompts)
  tkn-results config set --host=http://localhost:8080 --token=my-token

  # Configure with custom API path and namespace (no prompts)
  tkn-results config set --api-path=/api/v1 --namespace=my-namespace

  # Configure with custom kubeconfig and context
  tkn-results config set --kubeconfig=/path/to/kubeconfig --context=my-cluster

```
tkn-results config set
```

### Options

```
  -h, --help   help for set
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

* [tkn-results config](tkn-results_config.md)	 - Manage Tekton Results CLI configuration

