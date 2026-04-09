## tkn-results records

**[DEPRECATED]** Command sub-group for querying Records

> **Deprecation Notice**: This command is deprecated and will be removed in a future release.
> Use `tkn-results pipelinerun` or `tkn-results taskrun` commands instead.

### Options

```
  -h, --help   help for records
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
* [tkn-results records get](tkn-results_records_get.md)	 - **[DEPRECATED]** Get Record by <record-name> - use `pipelinerun describe` or `taskrun describe` instead
* [tkn-results records list](tkn-results_records_list.md)	 - **[DEPRECATED]** List Records for a given Result - use `pipelinerun list` or `taskrun list` instead

