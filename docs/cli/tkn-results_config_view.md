## tkn-results config view

Display current CLI configuration

### Synopsis

Display the current configuration settings for the Tekton Results CLI.

This command shows all configured settings including:
- API server endpoint
- Authentication token
- Cluster context and namespace
- TLS verification settings

The configuration is displayed in YAML format.

```
tkn-results config view
```

### Examples

```
View current configuration:
  tkn-results config view
```

### Options

```
  -h, --help   help for view
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

* [tkn-results config](tkn-results_config.md)	 - Manage Tekton Results CLI configuration

