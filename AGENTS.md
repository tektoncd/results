# Tekton Results

Result storage and queryable API for Tekton CI/CD workload history. Stores
TaskRun/PipelineRun/CustomRun execution data in persistent storage, freeing
cluster resources. Log retrieval is provided via pluggable logging
backends (e.g., Loki, Blob storage such as GCS/S3, Splunk).

**Behavioral guidelines**: See [.agents/guidelines.md](./.agents/guidelines.md) for generic coding principles.

---

## Build & Test Commands

```bash
# Build all binaries
make all

# Build specific component
make bin/api                    # API server
make bin/watcher                # Result watcher
make bin/retention-policy-agent # Retention policy agent
make bin/tkn-results            # CLI tool

# Test — requires no cluster
./test/presubmit-tests.sh --unit-tests

# Integration tests — requires kind cluster + Postgres
./test/presubmit-tests.sh --integration-tests

# Format code — required before PR submission
make fmt

# Lint — must pass before every PR
make golangci-lint

# Code generation — required after proto file changes
cd proto && go generate ./...
```

---

## Single-File Verification

```bash
# Lint a Go pkg
golangci-lint run path/to/package/

# Format single file
gofmt -w path/to/file.go

# Vet a Go pkg
go vet path/to/package/
```

---

## Key Conventions

1. **Postgres database is required.** The API server stores all execution data in Postgres.
   Development and testing require a running Postgres instance.

2. **Logs are not stored directly.** Tekton Results does not store logs in its database.
   Logs are fetched from a configured third-party logging backend (e.g.,
   Loki, Blob storage such as GCS/S3, Splunk) via the logging plugin.

3. **gRPC API is the source of truth.** All data mutations go through the API server
   at `pkg/api/server/`. The watcher and clients never write directly to the database.

4. **Proto definitions drive code generation.** After changing any `.proto` file in
   `proto/`, run `cd proto && go generate ./...` to regenerate Go bindings.

5. **Dependencies are vendored.** All Go dependencies live in `vendor/`. Review agents
   should ignore the `vendor/` directory — it contains third-party code.

6. **Use structured logging.** Import `knative.dev/pkg/logging` and use the context-aware
   logger. Never use `fmt.Printf` or `log.Print` in production code.

7. **Test coverage is enforced.** PRs that add functionality must include tests.
   Integration tests are tagged `//go:build e2e` and require a cluster.

---

## Architecture

**API Server** (`cmd/api`, `pkg/api/`): gRPC server backed by Postgres database.
Exposes Result and Record resources via proto-defined API (see `proto/<proto_version>/`).
Handles all data mutations and queries.

**Watcher** (`cmd/watcher`, `pkg/watcher/`): Kubernetes controller that watches
TaskRun, PipelineRun, and CustomRun resources. Creates or updates corresponding
Records via the Results API. Annotates original CRDs with result identifiers.

**Retention Policy Agent** (`cmd/retention-policy-agent`, `pkg/retention/`):
Deletes old data from the database based on configured retention policies.

**CLI** (`cmd/tkn-results`): Client tool for querying the Results API.

**Logs** (`pkg/logs/`): Integration with the configured logging backend
(e.g., Loki, Blob storage such as GCS/S3, Splunk) for log retrieval.
Logs are read from the external log store, not stored in the Results database.

---

## Pattern References for Common Changes

- **New API handler**: Follow the pattern in `pkg/api/server/<proto_version>/results.go`
- **Watcher shared reconciler logic**: See `pkg/watcher/reconciler/dynamic/`
- **Watcher resource-specific reconciler**: See `pkg/watcher/reconciler/pipelinerun/`, `taskrun/`, or `customrun/`
- **Proto definition changes**: Follow `proto/<proto_version>/results.proto`
- **Database migrations**: See `tools/tkn-results-migrator/`
- **Integration tests**: Follow examples in `test/e2e/`

---

## PR Conventions

- Pull requests must follow the repository PR template in `.github/pull_request_template.md`.
- Run `make fmt` before submitting for review.
- `make golangci-lint` must pass with zero issues.
- Tests required for any functionality changes.
- Follow [Tekton commit message standards](https://github.com/tektoncd/community/blob/main/standards.md#commits).
- Add `/kind <type>` label (bug, feature, cleanup, etc.).
- Update release notes block if user-facing changes.
- Run `cd proto && go generate ./...` after proto changes.
- Ignore `vendor/` directory in reviews — contains vendored dependencies only.

---

## Skills

None configured yet. Repo-local skills can be added to `.agents/skills/`.
