## tkn-results logs get

Get Log by <log-name>

### Synopsis

Get Log by <log-name>. <log-name> is typically of format <namespace>/results/<parent-run-uuid>/logs/<child-run-uuid>

```
tkn-results logs get [flags] <log-name>
```

### Examples

```
  Lets assume, there is a PipelineRun in 'default' namespace (parent) with:
  PipelineRun UUID: 0dfc883d-722a-4489-9ab8-3cccc74ca4f6 (parent)
  TaskRun 1 UUID: db6a5d59-2170-3367-9eb5-83f3d264ec62 (child 1)
  TaskRun 2 UUID: 9514f318-9329-485b-871c-77a4a6904891 (child 2)

  - Get the log for TaskRun 1:
    tkn-results logs get default/results/0dfc883d-722a-4489-9ab8-3cccc74ca4f6/logs/db6a5d59-2170-3367-9eb5-83f3d264ec62
  
  - Get log for TaskRun 2:
    tkn-results logs get default/results/0dfc883d-722a-4489-9ab8-3cccc74ca4f6/logs/9514f318-9329-485b-871c-77a4a6904891
  
  - Get log for the PipelineRun:
    tkn-results logs get default/results/0dfc883d-722a-4489-9ab8-3cccc74ca4f6/logs/0dfc883d-722a-4489-9ab8-3cccc74ca4f6
```

### Options

```
  -h, --help            help for get
  -o, --output string   output format. Valid values: textproto|json (default "json")
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

* [tkn-results logs](tkn-results_logs.md)	 - Commands for finding and retrieving logs

