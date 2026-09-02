# Postgres DB Layer E2E Tests

End-to-end tests that validate the Postgres-specific database layer of Tekton
Results. These tests run against the **deployed** API server in the kind e2e
cluster and its **live** Postgres instance—not an in-process server or a
throwaway database.

## What these tests cover

| Area | File | Status |
|---|---|---|
| Error mapping (pgconn.PgError → gRPC codes) | `error_mapping_test.go` | Active |
| Schema validation (column types, PKs, indexes, FKs, jsonb) | `schema_test.go` | Active |
| Lister behavior (filter, sort, pagination via Postgres) | `lister_test.go` | Active |
| Labels (normalized table, selector operators) | `labels_test.go` | Placeholder (Stories 11-13) |
| Metadata columns (text[], GIN indexes) | `migrations_test.go` | Placeholder (Stories 08-10) |
| golang-migrate migrations | `migrations_test.go` | Placeholder (Story 07) |
| Relationships & retention | `relationships_test.go`, `retention_test.go` | Placeholder (Stories 17, 20, 21) |

## Prerequisites

- A running kind cluster with Tekton Results installed (`./test/e2e/00-setup.sh`
  and `./test/e2e/01-install.sh`).
- The API server must be accessible at `localhost:8080` (the kind nodePort).
- Postgres must be reachable via `kubectl port-forward` (the `e2e.sh` script
  handles this automatically).

## Running

### Via the e2e script (CI path)

```bash
./test/e2e/e2e.sh
```

The script runs the standard e2e tests, then the DB layer tests, then the GCS
logging tests.

### Standalone (after cluster is up)

```bash
# Start port-forward to Postgres
kubectl port-forward svc/tekton-results-postgres-service 15432:5432 -n tekton-pipelines &

# Read the password
PGPASS=$(kubectl get secret tekton-results-postgres -n tekton-pipelines \
    -o jsonpath='{.data.POSTGRES_PASSWORD}' | base64 -d)

# Run (single DB_URL)
DB_URL="host=localhost port=15432 user=postgres password=${PGPASS} dbname=tekton-results sslmode=disable" \
go test -v -count=1 -tags=e2e ./test/e2e/db/...
```

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `DB_URL` | (none) | Full Postgres DSN. When set, takes precedence over individual `POSTGRES_*` vars. The `e2e.sh` script constructs this automatically. |
| `POSTGRES_HOST` | `localhost` | Fallback: Postgres host (via port-forward) |
| `POSTGRES_PORT` | `15432` | Fallback: local forwarded port |
| `POSTGRES_USER` | `postgres` | Fallback: DB user |
| `POSTGRES_PASSWORD` | (none) | Fallback: DB password (from cluster secret) |
| `POSTGRES_DB` | `tekton-results` | Fallback: database name (the live API database) |
| `POSTGRES_SSLMODE` | `disable` | Fallback: SSL mode |
| `SSL_CERT_PATH` | `/tmp/tekton-results/ssl` | Path to TLS cert for API |
| `SA_TOKEN_PATH` | `/tmp/tekton-results/tokens` | Path to SA token files |
| `API_SERVER_ADDR` | `https://localhost:8080` | API server address |
| `API_SERVER_NAME` | `tekton-results-api-service.tekton-pipelines.svc.cluster.local` | TLS server name |

## Adding tests for future stories

When your story lands a Postgres-specific feature:

1. Find the matching placeholder file (e.g., `labels_test.go`).
2. Replace the `t.Skip(...)` body with real test logic.
3. If no placeholder exists, create a new `*_test.go` file with the
   `//go:build e2e` tag.
4. Use `adminClient` for mutations, `readClient` for queries, and `rawDB`
   for direct schema introspection.
5. Clean up test data via the API in `t.Cleanup`—never drop or truncate
   tables (it is the live database).
