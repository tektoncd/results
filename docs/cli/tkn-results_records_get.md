## tkn-results records get

[To be deprecated] Get Record by <record-name>

### Synopsis

Get Record by <record-name>. <record-name> is typically of format <namespace>/results/<parent-run-uuid>/records/<child-run-uuid>

```
tkn-results records get [flags] <record-name>
```

### Examples

```
  Lets assume, there is a PipelineRun in 'default' namespace (parent) with:
  PipelineRun UUID: 0dfc883d-722a-4489-9ab8-3cccc74ca4f6 (parent)
  TaskRun 1 UUID: db6a5d59-2170-3367-9eb5-83f3d264ec62 (child 1)
  TaskRun 2 UUID: 9514f318-9329-485b-871c-77a4a6904891 (child 2)

  - Get the record for TaskRun 1:
    tkn-results records get default/results/0dfc883d-722a-4489-9ab8-3cccc74ca4f6/records/db6a5d59-2170-3367-9eb5-83f3d264ec62

  - Get the record for TaskRun 2:
    tkn-results records get default/results/0dfc883d-722a-4489-9ab8-3cccc74ca4f6/records/9514f318-9329-485b-871c-77a4a6904891

  - Get the record for PipelineRun:
    tkn-results records get default/results/0dfc883d-722a-4489-9ab8-3cccc74ca4f6/records/0dfc883d-722a-4489-9ab8-3cccc74ca4f6
```

### Options

```
  -h, --help            help for get
  -o, --output string   [To be deprecated] output format. Valid values: textproto|json (default "json")
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

* [tkn-results records](tkn-results_records.md)	 - [To be deprecated] Command sub-group for querying Records

