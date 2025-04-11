## tkn-results records list

List Records for a given Result

### Synopsis

List Records for a given Result. <result-name> is typically of format <namespace>/results/<parent-run-uuid>. '-' may be used in place of  <parent-run-uuid> to query all Records for a given parent.

```
tkn-results records list [flags] <result-name>
```

### Examples

```
  - List all Records for PipelineRun with UUID 0dfc883d-722a-4489-9ab8-3cccc74ca4f6 in 'default' namespace:
    tkn-results records list default/results/0dfc883d-722a-4489-9ab8-3cccc74ca4f6

  - List all Records for all Runs in 'default' namespace:
    tkn-results records list default/results/-
	
  - List only TaskRuns Records in 'default' namespace:
    tkn-results records list default/results/- --filter="data_type=='tekton.dev/v1beta1.TaskRun'"
```

### Options

```
  -f, --filter string   CEL Filter
  -h, --help            help for list
  -l, --limit int32     number of items to return. Response may be truncated due to server limits.
  -o, --output string   output format. Valid values: tab|textproto|json (default "tab")
  -p, --page string     pagination token to use for next page
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

* [tkn-results records](tkn-results_records.md)	 - Command sub-group for querying Records

