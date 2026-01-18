# Results API Server

## Variables

| Environment Variable     | Description                                                                                                                       | Example                                                                                      |
|--------------------------|-----------------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------|
| DB_USER                  | Postgres Database user                                                                                                            | user                                                                                         |
| DB_PASSWORD              | Postgres Database Password                                                                                                        | hunter2                                                                                      |
| DB_HOST                  | Postgres Database host                                                                                                            | /cloudsql/my-project:us-east1:tekton-results                                                 |
| DB_NAME                  | Postgres Database name                                                                                                            | tekton_results                                                                               |
| DB_SSLMODE               | Database SSL mode                                                                                                                 | verify-full                                                                                  |
| DB_SSLROOTCERT           | Path to CA cert used to validate Database cert                                                                                    | /etc/tls/db/ca.crt                                                                           |
| DB_ENABLE_AUTO_MIGRATION | Auto-migrate the database on startup (create/update schemas). For further details, refer to <https://gorm.io/docs/migration.html> | true (default)                                                                               |
| PROFILING                | Enable profiling server                                                                                                           | false  (default)                                                                             |
| PROFILING_PORT           | Profiling Server Port                                                                                                             | 6060  (default)                                                                              |
| DB_MAX_IDLE_CONNECTIONS  | The number of idle database connections to keep open                                                                              | 2 (default for golang, but specific database drivers may have settings for this too)         |
| DB_MAX_OPEN_CONNECTIONS  | The maximum number of database connections, for best performance it should equal DB_MAX_IDLE_CONNECTIONS                          | unlimited (default for golang, but specific database drivers may have settings for this too) |
| GRPC_WORKER_POOL         | The maximum number of goroutines pre-allocated for process GRPC requests. The GRPC server will also dynamically create threads.   | 2 (default)                                                                                  |
| K8S_QPS                  | The QPS setting for the kubernetes client created.                                                                                | 5 (default)                                                                                  |
| K8S_BURST                | The burst setting for the kubernetes client created.                                                                              | 10 (default)                                                                                 |
| SERVER_PORT              | gRPC and REST Server Port                                                                                                         | 8080  (default)                                                                              |
| PROMETHEUS_PORT          | Prometheus Port                                                                                                                   | 9090  (default)                                                                              |
| PROMETHEUS_HISTOGRAM     | Enable Prometheus histogram metrics to measure latency distributions of RPCs                                                      | false  (default)                                                                             |
| TLS_PATH                 | Path to TLS certificate files (tls.crt and tls.key)                                                                               | /etc/tls                                                                                     |
| TLS_MIN_VERSION          | Minimum TLS protocol version (e.g., "1.2", "1.3")                                                                                 | (Go's default)                                                                               |
| TLS_CIPHER_SUITES        | Comma-separated list of allowed cipher suites (IANA names or numeric IDs)                                                         | TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384                                                |
| TLS_CURVE_PREFERENCES    | Comma-separated list of elliptic curves for key exchange (e.g., X25519, P256)                                                     | X25519,P256                                                                                  |
| AUTH_DISABLE             | Disable RBAC check for resources                                                                                                  | false (default)                                                                              |
| AUTH_IMPERSONATE         | Enable RBAC impersonation                                                                                                         | true (default)                                                                               |
| LOG_LEVEL                | Log level for api server                                                                                                          | info (default)                                                                               |
| SQL_LOG_LEVEL                | Log level for gorm logger                                                                                                          | warn (default)                                                                               |
| LOGS_API                 | Enable logs storage service                                                                                                       | false (default)                                                                              |
| LOGS_TYPE                | Determine Logs storage backend type                                                                                               | File (default)                                                                               |
| LOGS_BUFFER_SIZE         | Buffer for streaming logs                                                                                                         | 32768 (default)                                                                              |
| LOGS_PATH                | Logs storage path                                                                                                                 | logs (default)                                                                               |
| LOGS_TIMESTAMPS          | Collect logs with timestamps                                                                                                      | false (default)                                                                              |
| S3_BUCKET_NAME           | S3 Bucket name                                                                                                                    | <S3 Bucket Name>                                                                             |
| S3_ENDPOINT              | S3 Endpoint                                                                                                                       | https://s3.ap-south-1.amazonaws.com                                                          |
| S3_HOSTNAME_IMMUTABLE    | S3 Hostname immutable                                                                                                             | false (default)                                                                              |
| S3_REGION                | S3 Region                                                                                                                         | ap-south-1                                                                                   |
| S3_ACCESS_KEY_ID         | S3 Access Key ID                                                                                                                  | <S3 Acces Key>                                                                               |
| S3_SECRET_ACCESS_KEY     | S3 Secret Access Key                                                                                                              | <S3 Access Secret>                                                                           |
| S3_MULTI_PART_SIZE       | S3 Multi part size                                                                                                                | 5242880 (default)                                                                            |
| GCS_BUCKET_NAME          | GCS Bucket Name                                                                                                                   | <GCS Bucket Name>                                                                            |
| STORAGE_EMULATOR_HOST    | GCS Storage Emulator Server                                                                                                       | http://localhost:9004                                                                        |
| CONVERTER_ENABLE         | Whether to start converter of v1beta1 TaskRun/PipelineRun records to v1                                                           | true                                                                                         |
| CONVERTER_DB_LIMIT       | How many records to convert at a time in a transaction                                                                            | 50                                                                                           |
| FEATURE_GATES            | Configuration to enable/disable a feature                                                                                         | PartialResponse=true,foo=false,bar=true                                                      |

These values can also be set in the config file located in the `config/env/config` directory.

Values derived from Postgres DSN

If you use the default postgres database we provide, the `DB_HOST` can be set as `tekton-results-postgres-service.tekton-pipelines`.

## TLS Configuration

The API server supports flexible TLS configuration through environment variables. This allows TLS settings to be managed externally (e.g., via Kubernetes ConfigMaps or the Tekton Operator) without requiring code changes.

### Configuration Options

- **TLS_MIN_VERSION**: Minimum TLS version (`1.0`, `1.1`, `1.2`, `1.3`). If not specified, Go's default is used.
- **TLS_CIPHER_SUITES**: Comma-separated cipher suites (IANA names or numeric IDs). If not specified, Go's default secure ciphers are used.
- **TLS_CURVE_PREFERENCES**: Comma-separated elliptic curves for key exchange. If not specified, Go's default curves are used.

### Supported Values

**TLS Versions:**
- `1.2` or `TLS1.2` - TLS 1.2
- `1.3` or `TLS1.3` - TLS 1.3 (recommended for PQC readiness)

**Cipher Suites (IANA names):**
- `TLS_AES_128_GCM_SHA256` (TLS 1.3)
- `TLS_AES_256_GCM_SHA384` (TLS 1.3)
- `TLS_CHACHA20_POLY1305_SHA256` (TLS 1.3)
- `TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256` (TLS 1.2)
- `TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384` (TLS 1.2)
- And other ciphers supported by Go's crypto/tls package

**Curve Preferences:**
- `X25519` - Modern curve (recommended)
- `P256` - NIST P-256
- `P384` - NIST P-384
- `P521` - NIST P-521
- `X25519Kyber768Draft00` - Post-Quantum Cryptography hybrid curve

### Example Configuration

```yaml
env:
  - name: TLS_MIN_VERSION
    value: "1.3"
  - name: TLS_CIPHER_SUITES
    value: "TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384,TLS_CHACHA20_POLY1305_SHA256"
  - name: TLS_CURVE_PREFERENCES
    value: "X25519,P256"
```

### Configuration Sources

TLS configuration can be provided through two sources:

1. **ConfigMap** (`tekton-results-api-config`): Values set in the configuration file mounted at `/etc/tekton/results/config`
2. **Environment Variables**: Values injected directly as container environment variables (e.g., by the Tekton Operator)

If neither is set, Go's default values are used.

#### All-or-Nothing Override Behavior

To prevent mixing settings from different sources that could result in incompatible TLS configurations (e.g., TLS 1.2 minimum version with TLS 1.3-only cipher suites), the API server uses an **all-or-nothing** approach:

- **If ANY TLS environment variable is set** (`TLS_MIN_VERSION`, `TLS_CIPHER_SUITES`, or `TLS_CURVE_PREFERENCES`), the API server uses **only environment variables** for all TLS settings. Unset variables will use Go's defaults.
- **If NO TLS environment variables are set**, the API server uses **only ConfigMap values** for all TLS settings.

This ensures that TLS configuration comes entirely from one source, avoiding partial overrides that could create invalid combinations.

The API server logs (debug level) which source is being used at startup:
- `"TLS configuration loaded from environment variables"` - using env vars
- `"TLS configuration loaded from config file"` - using ConfigMap

### OpenShift Integration

On OpenShift, the OpenShift Pipelines Operator can automatically configure these TLS settings based on the cluster's APIServer TLS Profile, enabling centralized TLS policy management. When the operator injects any TLS environment variable, it takes complete control of TLS configuration due to the all-or-nothing behavior described above.
