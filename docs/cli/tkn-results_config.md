## tkn-results config

Manage Tekton Results CLI configuration

### Synopsis

Manage configuration settings for the Tekton Results CLI.

This command allows you to configure how the CLI interacts with the Tekton Results API server.
You can set, view, and reset configuration values such as:
- API server endpoint
- Authentication token
- Cluster context and namespace
- TLS verification settings

Available subcommands:
  set    - Configure CLI settings
  view   - Display current configuration
  reset  - Reset configuration to defaults

Examples:
  # View current configuration
  tkn-results config view

  # Configure with automatic detection
  tkn-results config set

  # Configure with specific parameters
  tkn-results config set --host=http://localhost:8080 --token=my-token

  # Reset configuration to defaults
  tkn-results config reset

### Options

```
      --api-path string            api path to use (default: value provided in config set command)
  -c, --context string             name of the kubeconfig context to use (default: kubectl config current-context)
  -h, --help                       help for config
      --host string                host to use (default: value provided in config set command)
      --insecure-skip-tls-verify   skip server's certificate validation for requests (default: false)
  -k, --kubeconfig string          kubectl config file (default: $HOME/.kube/config)
  -n, --namespace string           namespace to use (default: from $KUBECONFIG)
      --token string               bearer token to use (default: value provided in config set command)
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

* [tkn-results](tkn-results.md)	 - Tekton Results CLI
* [tkn-results config reset](tkn-results_config_reset.md)	 - Reset CLI configuration to defaults
* [tkn-results config set](tkn-results_config_set.md)	 - Configure Tekton Results CLI settings
* [tkn-results config view](tkn-results_config_view.md)	 - Display current CLI configuration

