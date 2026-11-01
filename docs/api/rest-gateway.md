# REST Gateway Behavior and Telemetry

Results/Records REST requests invoke the same API server implementation used by native gRPC,
directly in process. Generated bindings retain the existing routes and JSON contract. The
existing service checker enforces authorization, including configured impersonation; the HTTP
logging layer adds no TokenReview or SubjectAccessReview calls. JWT identity fields in logs
remain unverified logging context and do not authorize requests.

The 4 MiB serialized-protobuf request limit and 100 MiB response limit remain in place.
Exceeding either retains ResourceExhausted/HTTP 429. These limits measure protobuf messages,
including decoded path/query/body fields; HTTP JSON bodies can be larger. Response size is
checked after `fields` selection, whose existing feature gate and nested-data behavior remain.
Cancellation and deadlines retain gateway status/error mapping. Panic recovery before a response
is committed returns the existing Unknown/HTTP 500 JSON body; committed streams are not rewritten.

`/healthz` uses the existing health server directly and preserves default/named service status
mapping and response format. Native gRPC service registration, interceptors and watcher traffic
retain their existing behavior.

## Logging modes

When LOGS_API is enabled without a supported plugin, the entire deprecated v1alpha2 Logs service
retains its gateway gRPC connection for streaming GetLog, ListLogs and DeleteLog. Startup logs
identify this exception. Client TLS options exist only for this legacy service. Plugins use the
existing HTTP handlers without gateway Logs registration. See [logging support](../logging-support.md).

## Prometheus migration

The existing metrics listener exposes these new families:

| Family | Meaning | Labels |
|--------|---------|--------|
| `tekton_results_rest_requests_total` | Completed matched gateway requests, including rejected/binding-error requests | `method`, `route`, `code` |
| `tekton_results_rest_request_duration_seconds` | Duration histogram, enabled by PROMETHEUS_HISTOGRAM | `method`, `route`, `code` |

`route` is a bounded matched template, not a resource name or namespace. `code` is the HTTP
status. Timing is always available in completion logs; the histogram flag controls distributions.
The middleware covers Results/Records, health, and registered legacy Logs. Plugin handlers and
unmatched routes retain their existing behavior outside these families.

Native `grpc_*` metric names and behavior remain unchanged. REST Results/Records and health
requests move from gRPC observations to these REST families. Update dashboards that previously
used gRPC families to represent REST traffic. For gateway REST request rates, use:

```promql
sum(rate(tekton_results_rest_requests_total[5m]))
```

A legacy REST Logs request contributes once to the REST counter and also to native Logs gRPC
counters through the retained connection. Native Logs counters therefore mix native callers
and REST loopback traffic. Do not sum those observations as independent client requests or
claim that existing gRPC labels identify their origin. Reconciliation is scoped to completed
matched REST gateway requests; a unique total across all public transports is not provided.
Streaming completion status is the HTTP status already committed; an error later in a legacy
stream remains visible through its error envelope and existing gRPC observations.

## Completion log migration

REST completion logs use `transport=rest` and the message `finished REST request`.

| Former REST reliance on gRPC fields | REST field |
|------------------------------------|------------|
| `grpc.method` | `http.method` and `http.route` |
| `grpc.code` | `http.status_code` |
| `grpc.time_duration_in_ms` | `http.duration_ms` |

`grpc.user`, `grpc.issuer`, and `grpc.auth_disabled` retain their identity/configuration meanings.
Tokens are not included in the new completion logs or metric labels. Health completion logs are
also available. A panic after response commitment is logged separately, while the completion
observation retains the committed status. Native gRPC log fields remain unchanged.

## Release note

Results/Records REST requests and REST health checks now dispatch in process, eliminating their
loopback gRPC connection while preserving API, authorization and payload-limit behavior. REST
requests gain separate HTTP metrics and completion-log fields; update dashboards as described
above. Deprecated legacy Logs REST streaming remains available through its existing connection
until a separate removal change. No measured performance gain is claimed by this release note.
