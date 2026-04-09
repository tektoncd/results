## tkn-results

Tekton Results CLI

### Synopsis

Command-line interface (CLI) designed to interact with Tekton Results. This CLI provides tools to configure how you interact with the Tekton Results API server, query TaskRuns and PipelineRuns and their associated data.

The following commands are supported:
  config        Manage CLI configuration for Results:
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

### Options

```
  -h, --help   help for tkn-results
```

### SEE ALSO

* [tkn-results config](tkn-results_config.md)	 - Manage Tekton Results CLI configuration
* [tkn-results pipelinerun](tkn-results_pipelinerun.md)	 - Query PipelineRuns
* [tkn-results taskrun](tkn-results_taskrun.md)	 - Query TaskRuns

