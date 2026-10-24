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

```
tkn-results config reset
```

### Examples

```
Reset all configuration settings:
  tkn-results config reset

Reset and verify the changes:
  tkn-results config reset && tkn-results config view

Reset and immediately reconfigure:
  tkn-results config reset && tkn-results config set
```

### Options

```
  -h, --help   help for reset
```

### Options inherited from parent commands

```
      --api-path string            api path to use (default: value provided in config set command)
      --as string                  username to impersonate for the operation (user or service account)
      --as-group stringArray       group to impersonate for the operation, this flag can be repeated to specify multiple groups
      --as-uid string              UID to impersonate for the operation
  -c, --context string             name of the kubeconfig context to use (default: kubectl config current-context)
      --host string                host to use (default: value provided in config set command)
      --insecure-skip-tls-verify   skip server's certificate validation for requests (default: false)
  -k, --kubeconfig string          kubectl config file (default: $KUBECONFIG, then $HOME/.kube/config)
  -n, --namespace string           namespace to use (default: the current kubeconfig context's namespace)
      --token string               bearer token to use (default: value provided in config set command)
```

### SEE ALSO

* [tkn-results config](tkn-results_config.md)	 - Manage Tekton Results CLI configuration

