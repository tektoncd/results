# Database migrations

Tekton Results stores its database schema as versioned SQL migrations in
[`pkg/api/server/db/migrations`](../pkg/api/server/db/migrations). The
migrations are compiled into the `results-admin` binary, which applies them.
The API server and the retention policy agent never change the schema. At
startup they only read the recorded schema version and refuse to start if it
is not compatible.

This page is for people who install and operate Tekton Results. Developers
changing the schema should read the
[developer guide](../pkg/api/server/db/migrations/README.md).

## How versions work

- Migrations are numbered and applied in order, one at a time, even within a
  single `migrate-up`. The database records the version of the last
  migration together with a `dirty` flag in the `schema_migrations` table.
  The flag is set while that migration runs and cleared when it completes.
- The **effective version** is the last migration that has fully completed:
  the recorded version when it is clean, and one less when it is dirty. For
  example, upgrading from 3 to 5 records 4 (dirty), 4, 5 (dirty), then 5;
  the effective version moves 3, 3, 4, 4, 5.
- Each release requires a minimum schema version (`RequiredSchemaVersion` in
  [`migrations.go`](../pkg/api/server/db/migrations/migrations.go)). A
  process starts only if the effective version is at or above it; otherwise
  it exits and is restarted until the migrations catch up.
- Migrations are additive, so a schema at a higher version keeps serving
  older releases. Several releases can share a schema, rolling back the
  application does not need a schema downgrade, and upgrading across several
  releases at once applies all pending migrations in one step.
- Most releases keep the required version of the release before them. A
  release that raises it says so in its release notes; see
  [Releases that raise the required version](#releases-that-raise-the-required-version).

## Installing and upgrading

Each release contains the `tekton-results-db-migrate` Job, which runs
`results-admin migrate-up`. Apply the release as usual:

```sh
kubectl apply -f release.yaml
kubectl wait job/tekton-results-db-migrate -n tekton-pipelines --for=condition=complete --timeout=10m
```

New API server pods exit with an explanatory message until the Job has
completed, and Kubernetes restarts them. Pods of the previous release keep
serving in the meantime.

If a pod of the Job fails, Kubernetes retries it in a new pod.
`kubectl logs job/tekton-results-db-migrate` shows the logs of one pod only;
to see all attempts, run:

```sh
kubectl logs -n tekton-pipelines -l job-name=tekton-results-db-migrate --prefix
```

A finished Job, with its pods and logs, is kept for six hours and then
deleted. A Job's pod template cannot be changed, so applying a different
release within that window fails with `spec.template: ... field is
immutable` for this Job only; everything else in the release is applied.
Replace just the Job from the release file, which deletes it and creates it
again, and runs the new release's `migrate-up`:

```sh
kubectl apply -f release.yaml -l app.kubernetes.io/name=tekton-results-db-migrate --force
kubectl wait job/tekton-results-db-migrate -n tekton-pipelines --for=condition=complete --timeout=10m
```

Always restrict `--force` to the Job with the label selector as shown; never
use it on the whole release file.
Do not replace the Job while a migration is running!

Rolling back to an earlier release within the window does not need the Job
at all: the earlier release runs against the newer schema, and applying its
release file fails only for the Job.

`migrate-up` is idempotent; running it again on an up-to-date database does
nothing.

### Releases that raise the required version

Most releases do not raise `RequiredSchemaVersion`, and their migrations can
fail or run long without affecting the running release. A release that does
raise it says so in its release notes. For such a release, if the migration
fails, no pod of the new release can start until the database is repaired
(see [Recovering from a failed migration](#recovering-from-a-failed-migration)):

- Applying `release.yaml` keeps the previous release's pods serving while
  the new ones restart, so the installation is degraded but available.
- The Tekton operator removes the previous release's Deployments before it
  creates the new ones, so the installation is unavailable until repaired.

To take the risk out of the upgrade itself, run the new release's
migrations first, while the previous release keeps serving, and apply the
release only after they completed:

```sh
kubectl apply -f release.yaml -l app.kubernetes.io/name=tekton-results-db-migrate --force
kubectl wait job/tekton-results-db-migrate -n tekton-pipelines --for=condition=complete --timeout=30m
kubectl apply -f release.yaml
```

With the operator, do not modify or replace `tekton-results-db-migrate`: the
operator owns it and reverts changes. Instead, before upgrading the
operator, run `results-admin migrate-up` with the new release's image from a
Job under a different name that the operator does not manage. The simplest
way is to copy the installed Job, which already carries the database
settings the operator applied, rename it, drop the labels and owner the
operator uses to recognise it, and swap in the new image (the
`results-admin` image reference is in the new release's `release.yaml`):

```sh
kubectl get job tekton-results-db-migrate -n tekton-pipelines -o json \
  | jq 'del(.status, .metadata.uid, .metadata.resourceVersion, .metadata.creationTimestamp,
            .metadata.generation, .metadata.managedFields, .metadata.ownerReferences,
            .metadata.labels, .metadata.annotations, .spec.selector,
            .spec.template.metadata.labels)
        | .metadata.name = "results-db-migrate-pre-upgrade"
        | .spec.template.spec.containers[0].image = "<results-admin image of the new release>"' \
  | kubectl apply -f -
kubectl wait job/results-db-migrate-pre-upgrade -n tekton-pipelines --for=condition=complete --timeout=30m
kubectl delete job results-db-migrate-pre-upgrade -n tekton-pipelines
```

Once it completes, upgrade the operator; the release's own Job then finds
nothing to do. If the installed Job has already been removed, take the Job
from the new release's `release.yaml` instead, rename it and remove its
labels before applying.

### Upgrading from a release without versioned migrations

Releases before versioned migrations created the schema on API server
startup, so their databases have the Results tables but no version history.
No extra step is needed: apply the new release as above. Before migrating,
the migration Job checks that the existing tables match the definition those
releases created and, if they do, records them as schema version 1. Existing
data is not changed, and API servers of the earlier release keep serving
until they are replaced.

If the tables differ, for example because the database was changed outside
Tekton Results, the Job fails and its log lists every difference:

```sh
kubectl logs job/tekton-results-db-migrate -n tekton-pipelines
```

Fix the reported differences, then re-create the Job from the release file
you installed, as described in
[Changing the database after installation](#changing-the-database-after-installation)
(the Job's template is unchanged, so `--force` would not re-create it).

## Administration

`results-admin` reads the same configuration as the API server (the
`tekton-results-api-config` ConfigMap mounted at `/etc/tekton/results`, plus
`DB_USER` and `DB_PASSWORD` from the environment).

| Command | Purpose |
|---|---|
| `status` | Print the recorded version, dirty flag, effective version, latest embedded version and the version this release requires, as JSON. |
| `check [--structural]` | Exit non-zero unless this release can run against the database. `--structural` also compares the baseline tables with their expected definition. |
| `migrate-up` | Apply all pending migrations. A database from a release without versioned migrations is adopted first if its tables match the baseline definition. |
| `migrate-to <version>` | Apply migrations up to `<version>`. Downgrades are refused. |
| `force <version> --repaired` | Mark a dirty schema clean after a failed migration was repaired by hand. |

Global flags bound every operation and are given before the command
(`results-admin --connect-timeout=20s migrate-up`); command flags such as
`--repaired` may be given before or after the command's argument.
`--connect-timeout` (default `10s`), `--lock-timeout` (default `30s`,
applies to the migration lock and to locks taken by schema changes), and
`--statement-timeout` (default unlimited).
Only one migration runs at a time; a second one waits for the lock up to
`--lock-timeout` and then fails. `--wait-for-database` (default `0`) keeps
retrying for the given duration while the database is not reachable yet,
for example while it is still starting; rejected logins and other errors
fail immediately. Before each retry it reads the configuration file again, so
database settings changed in the `tekton-results-api-config` ConfigMap while
it waits are used. Kubernetes updates a mounted ConfigMap within about a
minute; values passed as environment variables, such as the credentials from
the `tekton-results-postgres` Secret, are fixed for the life of the pod. The
migration Job uses `--wait-for-database=10m`, so it does not fail when it
starts before the database or before the database settings are in place.

## Changing the database after installation

Use this procedure when the database settings change after the release was
applied, for example when Tekton Results was installed with the default
settings and is then pointed at an external database, or when switching from
the bundled database to an external one. Data is not copied between
databases.

1. Change the settings in the `tekton-results-api-config` ConfigMap and the
   credentials in the `tekton-results-postgres` Secret.
2. Run the migration Job against the new database. If the Job from the
   installation is still waiting for the database and the credentials did
   not change, it picks up the new ConfigMap settings by itself; wait for it
   to complete. Otherwise, re-create only the Job from the release file you
   installed. Do not apply the whole file again: that would overwrite the
   ConfigMap with the release defaults.

   ```sh
   kubectl delete job tekton-results-db-migrate -n tekton-pipelines --ignore-not-found
   kubectl apply -f release.yaml -l app.kubernetes.io/name=tekton-results-db-migrate
   kubectl wait job/tekton-results-db-migrate -n tekton-pipelines --for=condition=complete --timeout=10m
   ```

   Use the same file you installed (`release.yaml` or `release_base.yaml`).
3. Restart the components that read the database settings at startup:

   ```sh
   kubectl rollout restart deployment tekton-results-api tekton-results-retention-policy-agent -n tekton-pipelines
   ```

Running the Job before the restart lets the existing pods keep serving from
the old database until the new one is ready, and the restarted pods start
without waiting.

## Recovering from a failed migration

golang-migrate records the version and sets `dirty` before it runs a
migration's SQL, so any interruption — a failed statement, a dropped
connection, a deleted pod — leaves the database dirty at the failed version
even when the migration's transaction was rolled back. `migrate-up` refuses
a dirty database, so the Job's retries fail fast and the state must be
repaired by hand:

- If the running release does not require the failed version (the usual
  case), it keeps running on the effective version and the repair is not
  urgent.
- If the release being installed requires the failed version, its pods do
  not start until the repair is done; see
  [Releases that raise the required version](#releases-that-raise-the-required-version).

Repair:

1. Inspect the failure in the migration Job's logs and the database.
2. Either complete the failed migration's changes by hand, or undo the parts
   it applied. Migrations are written to be idempotent, so undoing is only
   needed for objects that cannot be created twice: an interrupted
   `CREATE INDEX CONCURRENTLY` leaves an `INVALID` index behind
   (`\d <table>` in `psql` shows it); drop it before continuing.
3. Record the outcome:
   - completed: `results-admin force <failed version> --repaired`
   - undone: `results-admin force <failed version - 1> --repaired`
4. Run `results-admin migrate-up` again, for example by re-creating the Job
   as in [Changing the database after installation](#changing-the-database-after-installation).

`results-admin` runs with the API server's configuration. In the cluster,
run it from a one-off Pod that uses the `results-admin` image of the
release, the `tekton-results-api-config` ConfigMap and the
`tekton-results-postgres` Secret, as the Job's pod template does.

## Overriding the required version

If a release declares a higher required version than its code actually
needs and you cannot run its migrations yet, you can set
`DB_SCHEMA_REQUIRED_VERSION_OVERRIDE` in the API configuration to the version
to require instead. The API server and retention policy agent log a warning
on every start while the override is set. Remove it once the database has
been migrated. The override is compared with the effective version like the
required version is; it does not bypass an uninitialized database.

## Changing the schema

Schema changes are made by adding migration files to this repository, not
by editing the database. Migrations only add objects and never require
writers to stop, which is what lets an upgrade be a plain apply of the
release, and a rollback a plain apply of the previous one. Changes that
would need all writers on a newer release first, such as making a column
required, are not migrations; if one is ever needed it ships as a separate
`results-admin` command that checks its preconditions and is run on request,
and the release notes say so. The rules for writing a migration, raising
the required version and testing are in the developer guide at
[`pkg/api/server/db/migrations/README.md`](../pkg/api/server/db/migrations/README.md).
