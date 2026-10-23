## tkn-results config set

Set Tekton Results CLI configuration values

### Synopsis

Configure how the CLI connects to the Tekton Results API server.

Configuration Storage:
The configuration is stored in a namespace-independent way in your kubeconfig file.
This means the configuration persists across namespace switches (e.g., 'kubectl config 
set-context --current --namespace=production' or 'oc project production').
You only need to configure once per cluster/user combination.

Usage Modes:
1. Interactive: Prompts for values with defaults where available
   `tkn-results config set`

2. Manual: Specify values via flags
   `tkn-results config set --host=<url> --token=<token>`

Configuration Options:
- Host: Tekton Results API server URL
- Token: Bearer token. Leave blank to resolve automatically from your kubeconfig
  (recommended for exec/OIDC credentials, so it refreshes each request), or enter
  a static token explicitly to persist it.
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
      --as string                  username to impersonate for the operation (user or service account)
      --as-group stringArray       group to impersonate for the operation, this flag can be repeated to specify multiple groups
      --as-uid string              UID to impersonate for the operation
  -c, --context string             name of the kubeconfig context to use (default: kubectl config current-context)
  -h, --help                       help for set
      --host string                host to use (default: value provided in config set command)
      --insecure-skip-tls-verify   skip server's certificate validation for requests (default: false)
  -k, --kubeconfig string          kubectl config file (default: $KUBECONFIG, then $HOME/.kube/config)
  -n, --namespace string           namespace to use (default: the current kubeconfig context's namespace)
      --token string               bearer token to use (default: value provided in config set command)
```

### SEE ALSO

* [tkn-results config](tkn-results_config.md)	 - Manage Tekton Results CLI configuration

