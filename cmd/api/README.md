# Results API Server

## Variables

| Environment Variable | Description                | Example                                      |
|----------------------|----------------------------|----------------------------------------------|
| DB_USER              | Postgres Database user     | user                                         |
| DB_PASSWORD          | Postgres Database Password | hunter2                                      |
| DB_HOST              | Postgres Database host     | /cloudsql/my-project:us-east1:tekton-results |
| DB_NAME              | Postgres Database name     | tekton_results                               |
| DB_SSLMODE           | Database SSL mode          | verify-full                                  |
| GRPC_PORT            | gRPC Server Port           | 50051 (default)                              |
| REST_PORT            | REST proxy Port            | 8080  (default)                              |
| PROMETHEUS_PORT      | Prometheus Port            | 9090  (default)                              |
| LOG_LEVEL            | Log level for api server   | INFO                                         |
| TLS_HOSTNAME_OVERRIDE| Override the hostname used to serve TLS. This should not be set (or set to the empty string) in production environments.     | results.tekton.dev                           |
| TLS_PATH             | Path to TLS files          | /etc/tls                                     |

These values can also be set in the config file located in the `config/env/config` directory.

Values derived from Postgres DSN

If you use the default postgres database we provide, the `DB_HOST` can be set as `tekton-results-postgres-service.tekton-pipelines`.
