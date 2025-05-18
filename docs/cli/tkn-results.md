## tkn-results

Tekton Results CLI

### Synopsis

tkn-results CLI

tkn-results is a command-line interface (CLI) designed to interact with Tekton Results. This CLI provides tools to configure how you interact with the Tekton Results API server, query TaskRuns and PipelineRuns and their associated data.

The following commands are supported:
  config        Manage Tekton Results CLI configuration:
                - set:  Configure the CLI to connect to a Tekton Results instance.
                - view: Display the current CLI configuration.
                - reset: Reset the CLI configuration to defaults.
  taskrun       Query TaskRuns stored in Tekton Results:
                - list:  List TaskRun with filtering options.
                - describe:  Show detailed information about a specific TaskRun.
                - logs: Get logs for a TaskRun.
  pipelinerun   Query PipelineRuns stored in Tekton Results:
                - list:  List PipelineRuns with filtering options.
                - describe:  Show detailed information about a specific PipelineRun.
                - logs: Get logs for a PipelineRun.
Examples:
  tkn-results config set
  tkn-results config view
  tkn-results taskrun list -n default
  tkn-results pipelinerun describe my-pipelineRun

### Options

```
  -a, --addr string        [To be deprecated] Result API server address. If not specified, tkn-result would port-forward to service/tekton-results-api-service automatically
  -t, --authtoken string   [To be deprecated] authorization bearer token to use for authenticated requests
  -h, --help               help for tkn-results
      --insecure           [To be deprecated] determines whether to run insecure GRPC tls request
      --portforward        [To be deprecated] enable auto portforwarding to tekton-results-api-service, when addr is set and portforward is true, tkn-results will portforward tekton-results-api-service automatically (default true)
      --sa string          [To be deprecated] ServiceAccount to use instead of token for authorization and authentication
      --sa-ns string       [To be deprecated] ServiceAccount Namespace, if not given, it will be taken from current context
      --v1alpha2           [To be deprecated] use v1alpha2 API for get log command
```

### SEE ALSO

* [tkn-results config](tkn-results_config.md)	 - Manage Tekton Results CLI configuration
* [tkn-results logs](tkn-results_logs.md)	 - [To be deprecated] Commands for finding and retrieving logs
* [tkn-results pipelinerun](tkn-results_pipelinerun.md)	 - Query PipelineRuns
* [tkn-results records](tkn-results_records.md)	 - [To be deprecated] Command sub-group for querying Records
* [tkn-results result](tkn-results_result.md)	 - [To be deprecated] Query Results
* [tkn-results taskrun](tkn-results_taskrun.md)	 - Query TaskRuns

