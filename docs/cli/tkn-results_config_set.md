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
  --kubeconfig, -k          Path to kubeconfig file
  --context, -c             Kubernetes context to use
  --namespace, -n           Kubernetes namespace
  --api-path                API server path prefix
  --insecure-skip-tls-verify Skip TLS certificate verification

Note: When using configuration flags, you must also use --no-prompt to skip interactive prompts.

Examples:
  # Configure with automatic detection and interactive prompts
  tkn-results config set

  # Configure with specific parameters (must use --no-prompt)
  tkn-results config set --no-prompt --host=http://localhost:8080 --token=my-token

  # Configure with custom kubeconfig and context (must use --no-prompt)
  tkn-results config set --no-prompt --kubeconfig=/path/to/kubeconfig --context=my-cluster

```
tkn-results config set
```

### Options

```
  -h, --help        help for set
      --no-prompt   Skip interactive prompts and use default values
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

* [tkn-results config](tkn-results_config.md)	 - Manage Tekton Results CLI configuration

