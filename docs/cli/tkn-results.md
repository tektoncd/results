## tkn-results

Tekton Results CLI

### Synopsis

Environment Variables:
    TKN_RESULTS_SSL_ROOTS_FILE_PATH: Path to local SSL cert to use.
    TKN_RESULTS_SSL_SERVER_NAME_OVERRIDE: SSL server name override (useful if using with a proxy such as kubectl port-forward).

Config:
    A config file may be stored in `~/.config/tkn/results.yaml` to configure the CLI client.

    Fields:
    - address: Results API Server address
    - service_account: When specified, the CLI will first fetch a bearer token
                       for the specified ServiceAccount and attach that to Result API requests.
        - namespace: ServiceAccount namespace
        - name: ServiceAccount name
    - token: Bearer token to use for API requests. Takes priority over service_account.
    - ssl: SSL connection options
        - roots_file_path: Path to a certificate to include in the cert pool. Useful for adding allowed self-signed certs.
        - server_name_override: For testing only. Sets the grpc.ssl_target_name_override value for requests.
    - portforward: enable auto portforwarding to tekton-results-api-service when address is set and portforward is true, tkn-results will portforward tekton-results-api-service automatically

Example:

    ```
    address: results.dogfooding.tekton.dev:443
    token: abcd1234
    ssl:
        roots_file_path: path/to/file
        server_name_override: example.com
    service_account:
        namespace: default
        name: result-reader
    portforward: false
    ```



### Options

```
  -a, --addr string        Result API server address. If not specified, tkn-result would port-forward to service/tekton-results-api-service automatically
  -t, --authtoken string   authorization bearer token to use for authenticated requests
  -h, --help               help for tkn-results
      --insecure           determines whether to run insecure GRPC tls request
      --portforward        enable auto portforwarding to tekton-results-api-service, when addr is set and portforward is true, tkn-results will portforward tekton-results-api-service automatically (default true)
      --sa string          ServiceAccount to use instead of token for authorization and authentication
      --sa-ns string       ServiceAccount Namespace, if not given, it will be taken from current context
      --v1alpha2           use v1alpha2 API for get log command
```

### SEE ALSO

* [tkn-results config](tkn-results_config.md)	 - Manage Tekton Results CLI configuration
* [tkn-results logs](tkn-results_logs.md)	 - Commands for finding and retrieving logs
* [tkn-results pipelinerun](tkn-results_pipelinerun.md)	 - Query PipelineRuns
* [tkn-results records](tkn-results_records.md)	 - Command sub-group for querying Records
* [tkn-results result](tkn-results_result.md)	 - Query Results

