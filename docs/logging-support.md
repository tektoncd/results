# Tekton Results Logging Support

## Overview

Tekton Results supports fetching logs from third party logging APIs.

To enable the logging API, in the results-api-configmap set `LOGS_API` to `true` and
and `LOGS_TYPE` to the provider type (see below).

## Loki

The following environment variables are required:

- `LOGS_TYPE`: Set to `Loki` to enable the fetching of logs from Loki.

## GCS or S3

The following environment variables are required:

- `LOGS_TYPE`: Set to `Blob` to enable the fetching of logs from GCS or S3.
- `LOGS_PATH`: Directory under bucket where logs are stored.
- `LOGGING_PLUGIN_API_URL`: The URL of the bucket accepted by GO CDK library, e.g: s3://tekton-logs
- `LOGGING_PLUGIN_QUERY_PARAMS`: Query params to configure Blob library. We also have a query param `legacy` for enabling backward compatibility with legacy get log API. You can set `legacy=true`. 

You can check a configurations for minio s3:
Vector Minio: https://github.com/tektoncd/results/blob/main/test/e2e/blob-logs/vector-s3.yaml
API Config: https://github.com/tektoncd/results/blob/main/test/e2e/blob-logs/vector-minio-config.yaml
Next, you can volume mount following the secret and set AWS_SHARED_CREDENTIALS_FILE and AWS_CONFIG_FILE.

```
[default]
aws_access_key_id = Q3AM3UQ867SPQQA43P2F
aws_secret_access_key = zuf+tfteSlswRu7BJ86wekitnifILbZam1KYY3TG
region = us-east-1
endpoint_url = https://play.min.io:9000
```

## Common Configuration

These are the common configuration options for all third party logging APIs.

- `LOGS_API`: Set to `true` to enable the Logs API.
- `LOGGING_PLUGIN_API_URL`: The URL of the third party logging API.
- `LOGGING_PLUGIN_TOKEN_PATH`: The path to the file containing the API token. (optional)
- `LOGGING_PLUGIN_NAMESPACE_KEY`: The key to use for the namespace filtering.
- `LOGGING_PLUGIN_STATIC_LABELS`: The static labels to use for the logs.
- `LOGGING_PLUGIN_PROXY_PATH`: The path to the proxy to use for the third party logging API. (optional)
- `LOGGING_PLUGIN_CA_CERT`: The CA certificate to use for the third party logging API. This should ideally be passed as environment variable in the deployment spec of the results-api pod. (optional)
- `LOGGING_PLUGIN_TLS_VERIFICATION_DISABLE`: Set to `true` to disable TLS verification for the third party logging API. (optional)
- `LOGGING_PLUGIN_FORWARDER_DELAY_DURATION`: This is the max duration in minutes taken by third party logging system to forward and store the logs after completion of taskrun and pipelinerun. This is used to search between start time of runs and completion plus buffer duration.
- `LOGGING_PLUGIN_QUERY_LIMIT`: Sets the query limit for Third Party Logging API if logging backend has a limit on number of log lines returned.
- `LOGGING_PLUGIN_QUERY_PARAMS`: Sets the query params for Third Party Logging API, these can be direction/sort order.Specify them in this format: "foo=bar&direction=backward"
