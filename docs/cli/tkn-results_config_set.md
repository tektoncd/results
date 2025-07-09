## tkn-results config set

Configure Tekton Results CLI settings

### Synopsis

Configure settings for the Tekton Results CLI.

This command allows you to configure how the CLI interacts with the Tekton Results API server.
It can automatically detect the API server in OpenShift environments or allow manual configuration.

The command will:
1. Automatically detect the Tekton Results API server in OpenShift environments
2. Prompt for any missing configuration values
3. Save the configuration for future use

Detection Strategy:
- OpenShift: Automatically detects routes in openshift-pipelines and tekton-results namespaces
- Kubernetes: Manual configuration required (automatic detection not available)

Examples:
  # OpenShift: Automatic detection
  tkn-results config set

  # Kubernetes: Manual configuration (required)
  tkn-results config set --host=<api-server-url> --token=<token> --api-path=<path>

  # Manual configuration with custom settings
  tkn-results config set --host=<api-server-url> --token=<token> --api-path=/api/v1 --insecure-skip-tls-verify

Automatic Detection (OpenShift only):
- Detects routes in openshift-pipelines and tekton-results namespaces
- Constructs API URLs from route configuration
- Uses service account token (if available)
- Filters routes by service name for better accuracy

Manual Configuration (Kubernetes or custom):
- API server host URL
- Authentication token
- API path prefix
- TLS verification settings

If automatic detection fails in OpenShift, you can provide values manually using the available flags.

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

