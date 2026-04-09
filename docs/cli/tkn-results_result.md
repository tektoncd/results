## tkn-results result

**[DEPRECATED]** Query Results

> **Deprecation Notice**: This command is deprecated and will be removed in a future release.
> Use `tkn-results pipelinerun` or `tkn-results taskrun` commands instead.

### Options

```
  -h, --help   help for result
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
* [tkn-results result describe](tkn-results_result_describe.md)	 - **[DEPRECATED]** Describes a Result - use `pipelinerun describe` or `taskrun describe` instead
* [tkn-results result list](tkn-results_result_list.md)	 - **[DEPRECATED]** List Results - use `pipelinerun list` or `taskrun list` instead

