## tkn-results logs

**[DEPRECATED]** Commands for finding and retrieving logs

> **Deprecation Notice**: This command is deprecated and will be removed in a future release.
> Use `tkn-results pipelinerun logs` or `tkn-results taskrun logs` commands instead.

### Options

```
  -h, --help   help for logs
```

### Options inherited from parent commands

```
  -a, --addr string        [DEPRECATED] Result API server address. Use 'config set --host=<host>' instead
  -t, --authtoken string   [DEPRECATED] authorization bearer token. Use 'config set --token=<token>' instead
      --insecure           [DEPRECATED] determines whether to run insecure GRPC tls request. Use 'config set --insecure' instead
      --portforward        [DEPRECATED] enable auto portforwarding. Use 'config set' instead (default true)
      --sa string          [DEPRECATED] ServiceAccount for authorization. Use 'config set' instead
      --sa-ns string       [DEPRECATED] ServiceAccount Namespace. Use 'config set' instead
      --v1alpha2           [DEPRECATED] use v1alpha2 API. This flag is no longer needed
```

### SEE ALSO

* [tkn-results](tkn-results.md)	 - Tekton Results CLI
* [tkn-results logs get](tkn-results_logs_get.md)	 - **[DEPRECATED]** Get Log by <log-name> - use `pipelinerun logs` or `taskrun logs` instead
* [tkn-results logs list](tkn-results_logs_list.md)	 - **[DEPRECATED]** List Logs for a given Result - use `pipelinerun logs` or `taskrun logs` instead

