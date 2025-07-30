## tkn-results config set

Set Tekton Results CLI configuration values

### Synopsis

Configure settings for the Tekton Results CLI.

This command allows you to configure how the CLI interacts with the Tekton Results API server.
It can automatically detect the API server in your cluster or allow manual configuration.

The command will:
1. Automatically detect platform type (OpenShift vs Kubernetes)
2. Search for routes (OpenShift) or ingresses (Kubernetes) in the appropriate namespace
3. Prompt for any missing configuration values
4. Save the configuration for future use

Detection Strategy:
- OpenShift: Automatically detects routes in openshift-pipelines namespace (default) or custom namespace
- Kubernetes: Automatically detects ingresses in tekton-pipelines namespace (default) or custom namespace

Examples:
  # Automatic detection with default namespace
  tkn-results config set

  # Automatic detection with custom Results namespace
  tkn-results config set --results-namespace=my-tekton-namespace

  # Manual configuration (when automatic detection fails)
  tkn-results config set --host=<api-server-url> --token=<token> --api-path=<path>

  # Manual configuration with custom settings
  tkn-results config set --host=<api-server-url> --token=<token> --api-path=/api/v1 --insecure-skip-tls-verify

Automatic Detection:
- OpenShift: Detects routes in openshift-pipelines namespace (default) or user-provided namespace
- Kubernetes: Detects ingresses in tekton-pipelines namespace (default) or user-provided namespace
- Constructs API URLs from route/ingress configuration
- Filters routes/ingresses by service name (tekton-results-api-service) for better accuracy

Manual Configuration (when automatic detection fails):
- API server host URL
- Authentication token
- API path prefix
- TLS verification settings

If automatic detection fails or RBAC permissions are insufficient, you can provide values manually using the available flags.

Results Namespace:
The --results-namespace flag allows you to specify where Tekton Results is installed:
- Default: tekton-pipelines (Kubernetes) or openshift-pipelines (OpenShift)
- Custom: Use --results-namespace to specify a different namespace

Permission Requirements:
- Namespace access (get namespaces)
- Route access (OpenShift: get routes)
- Ingress access (Kubernetes: get ingresses)
If you encounter permission errors, ask your admin to setup RBAC or use manual configuration.

```
tkn-results config set
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
      --results-namespace string   namespace where Tekton Results is installed (default: tekton-pipelines for K8s, openshift-pipelines for OpenShift)
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

