## tkn-results result list

[To be deprecated] List Results

```
tkn-results result list [flags] <parent>

  <parent>: Parent name to query. This is typically corresponds to a namespace, but may vary depending on the API Server. "-" may be used to query all parents. This will list results for namespaces the token has access to
```

### Options

```
  -f, --filter string   [To be deprecated] CEL Filter
  -h, --help            help for list
  -l, --limit int32     [To be deprecated] number of items to return. Response may be truncated due to server limits.
  -o, --output string   [To be deprecated] output format. Valid values: tab|textproto|json (default "tab")
  -p, --page string     [To be deprecated] pagination token to use for next page
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

* [tkn-results result](tkn-results_result.md)	 - [To be deprecated] Query Results

