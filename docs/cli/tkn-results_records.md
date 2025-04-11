## tkn-results records

Command sub-group for querying Records

### Options

```
  -h, --help   help for records
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
* [tkn-results records get](tkn-results_records_get.md)	 - Get Record by <record-name>
* [tkn-results records list](tkn-results_records_list.md)	 - List Records for a given Result

