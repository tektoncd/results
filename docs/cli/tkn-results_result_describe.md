## tkn-results result describe

Describes a Result

```
tkn-results result describe [-p parent -u uid] [name]
```

### Examples

```
Query by name:
tkn-results result describe default/results/e6b4b2e3-d876-4bbe-a927-95c691b6fdc7

Query by parent and uid:
tkn-results result desc --parent default --uid 949eebd9-1cf7-478f-a547-9ee313035f10

```

### Options

```
  -h, --help            help for describe
  -p, --parent string   parent of the result
  -u, --uid string      uid of the result
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

* [tkn-results result](tkn-results_result.md)	 - Query Results

