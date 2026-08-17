# Horizontal Scaling

Tekton Results supports horizontal scaling for both the API server and the watcher to provide high availability and increased throughput.

## Overview

Horizontal scaling enables Tekton Results to:

- **Increase availability** - Multiple replicas eliminate single points of failure
- **Distribute load** - Workload is partitioned across watcher replicas and API requests are load-balanced across API replicas
- **Scale throughput** - More replicas handle more concurrent reconciliations and API requests

The horizontal scaling configuration deploys:

- **3 API server replicas** behind a headless Service for DNS-based gRPC load balancing
- **3 watcher replicas** as a StatefulSet with bucket-based workload partitioning
- **3 buckets** for consistent hashing of Tekton resources across watchers

## Architecture

### API Server Scaling

The API server scales horizontally using a standard Kubernetes Deployment with multiple replicas. Load balancing is achieved through DNS-based gRPC client-side load balancing.

**Components:**

- **Deployment** - `tekton-results-api` with 3 replicas
- **Headless Service** - `tekton-results-api-service-headless` with `clusterIP: None`
- **ClusterIP Service** - `tekton-results-api-service` for non-HA clients (preserved for backwards compatibility)

**How it works:**

1. The headless Service has `clusterIP: None`, causing DNS queries to return all pod IP addresses instead of a single virtual IP
2. gRPC clients use the `dns:///` scheme to trigger DNS-based endpoint resolution
3. The `round_robin` load balancing policy distributes requests evenly across all resolved endpoints
4. Each watcher maintains a single gRPC connection that multiplexes requests to all API replicas via HTTP/2 streams

**Why headless Service:**

Standard Kubernetes ClusterIP Services load-balance at the TCP connection level. Since gRPC uses long-lived HTTP/2 connections, all requests from a single watcher would go to the same API pod. The headless Service enables DNS to return all pod IPs, allowing gRPC's client-side load balancer to distribute individual requests across all API replicas.

### Watcher Scaling

The watcher scales horizontally using a StatefulSet with bucket-based workload partitioning provided by Knative's leader-aware controllers.

**Components:**

- **StatefulSet** - `tekton-results-watcher` with 3 replicas (replaces the base Deployment)
- **Headless Service** - `tekton-results-watcher-headless` for StatefulSet network identity
- **ConfigMap** - `config-leader-election` with `buckets: "3"`

**How it works:**

1. Knative's controller framework partitions the reconciliation key space into buckets using consistent hashing
2. Each StatefulSet replica is assigned one or more buckets based on its ordinal (derived from pod name)
3. When a watcher observes a TaskRun, PipelineRun, or CustomRun, it computes the bucket hash from the resource's namespace/name
4. The watcher only reconciles the resource if the computed bucket matches one of its assigned buckets
5. Resources are deterministically assigned to exactly one watcher, eliminating duplicate reconciliation

Note: Deprecated blocking grpc.DialContext client connection function has been replaced with newer grpc.NewClient function. grpc.WithBlock is uncompatible with the new scaling functionality and has been removed and replaced with grpc.WithConnectParams(connectParams) to use new scaling parameters;


**Bucket assignment example:**

With 3 replicas and 3 buckets:
- `tekton-results-watcher-0` reconciles bucket 0
- `tekton-results-watcher-1` reconciles bucket 1
- `tekton-results-watcher-2` reconciles bucket 2

A TaskRun named `default/my-task` hashes to bucket 1, so only `watcher-1` reconciles it.

**StatefulSet vs Deployment:**

StatefulSets provide stable network identities and ordinals that Knative uses for bucket assignment. The `STATEFUL_CONTROLLER_ORDINAL` environment variable (set by Knative injection) identifies which replica this is, enabling the bucket-to-replica mapping.

## Configuration

### Deploying with Kustomize

The horizontal scaling feature is packaged as a Kustomize component at `config/components/horizontal-scaling`.

**Example overlay:**

```yaml
# config/overlays/ha-local-db/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: tekton-pipelines
resources:
  - ../../base
components:
  - ../../components/local-db
  - ../../components/horizontal-scaling
  - ../../components/metadata
```

**Deploy:**

```bash
# Generate TLS certificate with both service names
./config/components/horizontal-scaling/generate-tls-cert.sh

# Deploy with kustomize and ko
kubectl kustomize config/overlays/ha-local-db | ko apply -f -
```

### Environment Variables

**Watcher StatefulSet:**

- `TEKTON_RESULTS_API_SERVICE` - API server address using DNS scheme for load balancing
  - Value: `dns:///tekton-results-api-service-headless.tekton-pipelines.svc.cluster.local:8080`
  - The `dns:///` prefix triggers DNS-based endpoint resolution
- `AUTH_MODE` - Authentication mode for API requests
  - Value: `token`
  - Uses TLS transport credentials with per-RPC service account token authentication
  - Do NOT use `insecure` in production - it disables TLS encryption entirely
- `SYSTEM_NAMESPACE` - Namespace for Knative configuration
  - Value: Derived from pod metadata (`metadata.namespace`)
- `STATEFUL_CONTROLLER_ORDINAL` - Pod ordinal for bucket assignment
  - Value: Derived from pod name (`metadata.name`)
  - Knative extracts the ordinal number (0, 1, 2) from the StatefulSet pod name
- `STATEFUL_SERVICE_NAME` - Headless Service name for StatefulSet
  - Value: `tekton-results-watcher-headless`

**ConfigMap (config-leader-election):**

- `buckets` - Number of buckets for workload partitioning
  - Value: `"3"` (must be a string)
  - Must equal the number of watcher replicas
  - Maximum supported by Knative: 10

### Scaling to Different Replica Counts

To scale to a different number of replicas:

1. Update `spec.replicas` in `config/components/horizontal-scaling/watcher-statefulset.yaml`
2. Update the `buckets` value in `config/components/horizontal-scaling/kustomization.yaml`
3. Optionally update API replicas in the same kustomization file

**Example for 5 replicas:**

```yaml
# watcher-statefulset.yaml
spec:
  replicas: 5

# kustomization.yaml patches
patches:
  - target:
      kind: Deployment
      name: api
    patch: |-
      - op: replace
        path: /spec/replicas
        value: 5
  - target:
      kind: ConfigMap
      name: config-leader-election
    patch: |-
      - op: add
        path: /data/buckets
        value: "5"
```

### TLS Certificate Requirements

The TLS certificate must include both the ClusterIP Service name and the headless Service name in the Subject Alternative Names (SAN):

1. `tekton-results-api-service.tekton-pipelines.svc.cluster.local`
2. `tekton-results-api-service-headless.tekton-pipelines.svc.cluster.local`

Without both names, watcher pods will fail TLS validation when connecting via the headless Service.

**Generate certificate:**

```bash
./config/components/horizontal-scaling/generate-tls-cert.sh
```

**Verify certificate includes both names:**

```bash
kubectl get secret -n tekton-pipelines tekton-results-tls \
  -o jsonpath='{.data.tls\.crt}' | base64 -d \
  | openssl x509 -noout -text | grep -A1 "Subject Alternative Name"
```

## gRPC Connection

### grpc.NewClient Migration

The watcher uses `grpc.NewClient` (introduced in gRPC-Go 1.58) instead of the deprecated `grpc.DialContext`.

**Key differences:**

| `grpc.DialContext` (deprecated) | `grpc.NewClient` (current) |
|--------------------------------|---------------------------|
| Blocking by default | Non-blocking - returns immediately |
| Accepts `context.Context` | No context parameter |
| Supports `grpc.WithBlock()` | Does not support `grpc.WithBlock()` |
| Connection established during dial | Connections established lazily on first RPC |


### DNS-Based Load Balancing

The watcher configures the gRPC client with DNS-based endpoint resolution and round-robin load balancing.

**How DNS resolution works:**

1. gRPC parses the `dns:///` scheme and extracts the hostname
2. The DNS resolver queries the headless Service name
3. Kubernetes DNS returns A records for all ready API pod IPs
4. gRPC creates subchannels to each resolved endpoint
5. The `round_robin` picker distributes RPCs across all subchannels in order

**Re-resolution:**

gRPC re-resolves DNS every 30 minutes by default. New API pods may take up to 30 minutes to receive traffic from existing watcher connections. Manual watcher restart forces immediate re-resolution.

### Manual Testing

**Verify watcher bucket assignments:**

```bash
kubectl logs -n tekton-pipelines tekton-results-watcher-0 | grep "knative.dev/key"
kubectl logs -n tekton-pipelines tekton-results-watcher-1 | grep "knative.dev/key"
kubectl logs -n tekton-pipelines tekton-results-watcher-2 | grep "knative.dev/key"
```

Each watcher should show log entries for a distinct subset of resources.

**Verify API load balancing:**

```bash
kubectl logs -n tekton-pipelines -l app.kubernetes.io/name=tekton-results-api | grep "grpc.method" | wc -l
```

All API pods should have non-zero request counts.

**Verify no duplicate records:**

```bash
tkn-results records list --result default/results/<result-id>
```

Should return exactly 1 TaskRun record (plus any log/event records).

## Operational Considerations

### Scaling Down

Bucket ownership is assigned statically at startup (each pod computes its
buckets from its StatefulSet ordinal and the configured `buckets` count) and
is never reassigned dynamically. Scaling down requires all of the following
steps:

1. Update `spec.replicas` in the watcher StatefulSet
2. Update `buckets` in the config-leader-election ConfigMap to match the new replica count
3. Apply the updated configuration
4. Restart (roll) the remaining watcher pods so they read the updated `buckets` value and recompute their bucket assignments

Steps 2-4 are all required: the leader election config is only read once at
process startup, so a running pod never picks up a `buckets` change on its
own. Only a restart with the corrected value causes a pod to recompute its
assigned buckets.

**Safe scale-down:**

Kubernetes deletes StatefulSet pods in reverse ordinal order (2, 1, 0); this
is an independent, unrelated fact from bucket reassignment, which is a
config value read once at process start, not a reaction to pod deletion. If
`buckets` is not updated and the remaining pods are not restarted, the
buckets previously owned by the removed pods become orphaned: no pod owns
them, and resources hashing into those buckets go unreconciled until the
misconfiguration is corrected.

### Rolling Updates

When updating the watcher StatefulSet (new image, config change):

1. Kubernetes performs a rolling update, replacing one pod at a time
2. Knative demotes the terminating pod's buckets and promotes a different replica
3. The new pod starts, cache syncs, and takes ownership of assigned buckets
4. Reconciliations continue with at-least-once delivery guarantees

**Downtime:**

Individual resources may experience reconciliation delays during rolling updates (up to the requeue interval), but no reconciliations are lost.

### DNS Re-Resolution Latency

gRPC-Go's built-in DNS resolver does not poll on a timer. It only re-resolves
reactively, when a connection fails (e.g. a subchannel hits
`TRANSIENT_FAILURE`), subject to a 30-second minimum interval between lookups
(`MinResolutionInterval`). A healthy connection has no failure to react to, so
it may never discover newly added API replicas on its own.

**Force immediate re-resolution:**

```bash
kubectl rollout restart statefulset/tekton-results-watcher -n tekton-pipelines
```

New watcher pods establish fresh connections with current DNS resolution.

### Maximum Bucket Count

Knative's leader-aware controller framework supports a maximum of 10 buckets. Scaling beyond 10 replicas requires code changes to the bucket assignment algorithm.

For most deployments, 3-5 replicas provide sufficient availability and throughput.
