## tkn-results config reset

Reset CLI configuration to defaults

### Synopsis

Reset all configuration settings to their default values.

This command will:
1. Remove all custom configuration settings
2. Reset to default values:
   - API server endpoint
   - Authentication token
   - Cluster context and namespace
   - TLS verification settings

Warning: This will remove all custom configuration settings.
         You will need to reconfigure the CLI after resetting.

Examples:
  # Reset all configuration settings
  tkn-results config reset

  # Reset and verify the changes
  tkn-results config reset && tkn-results config view

  # Reset and immediately reconfigure
  tkn-results config reset && tkn-results config set

```
tkn-results config reset
```

### Options

```
  -h, --help   help for reset
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

