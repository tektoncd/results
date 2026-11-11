# Schema migrations: developer guide

This package owns the Tekton Results database schema. It holds the numbered
SQL migrations, the code that applies them (used by the `results-admin`
binary in [`cmd/results-admin`](../../../../../cmd/results-admin)) and the
startup check that the API server and retention policy agent run. For how
operators install, upgrade and repair a database, see the user documentation
in [`docs/database-migrations.md`](../../../../../docs/database-migrations.md);
this file is about changing the schema.

## Layout

| File | Role |
|---|---|
| `NNN_name.up.sql`, `NNN_name.down.sql` | Migrations, embedded with `go:embed` and applied in order by [golang-migrate](https://github.com/golang-migrate/migrate) through its `pgx/v5` driver. `001_baseline` is the schema that GORM AutoMigrate created in earlier releases. |
| `migrations.go` | `RequiredSchemaVersion`, the `schema_migrations` reader and the compatibility check (`Check`, `CheckAtStartup`). Read-only; never creates or locks anything. |
| `admin.go` | `MigrateUp`, `MigrateTo`, `Force`, connection handling and the advisory lock that serialises runners. Adoption of a legacy database happens here. |
| `structure.go` | The expected shape of the baseline tables, used to adopt a legacy database and by `check --structural`. Frozen; see below. |
| `migrations_test.go` | Layout, lint and unit tests, no database needed. |
| `postgres_test.go` | Tests against a real PostgreSQL (`-tags=e2e`, `DB_URL`). |

## How a migration runs

- golang-migrate records `version = N, dirty = true` in `schema_migrations`
  *before* executing `NNN.up.sql` and clears `dirty` afterwards. Each file
  is sent to PostgreSQL as one `Exec`, so a file with several statements
  runs in one implicit transaction and is rolled back as a whole on error.
  A file whose single statement cannot run in a transaction
  (`CREATE INDEX CONCURRENTLY`) must contain nothing else.
- Any interruption leaves the database dirty at `N`, even when the
  transaction was rolled back. `migrate-up` refuses a dirty database; the
  operator repairs it with `force`. This is why migrations must be
  idempotent: repair is then always `force <N-1> --repaired` + `migrate-up`.
- The **effective version** used by the startup check is `version` when
  clean and `version - 1` when dirty, so code never depends on a migration
  that has not completed.
- Only one runner migrates at a time (golang-migrate advisory lock, bounded
  by `--lock-timeout`, which is also PostgreSQL `lock_timeout` for the DDL).

## Adding a migration

1. Add `NNN_name.up.sql` and `NNN_name.down.sql`, where `NNN` is the next
   version. Versions are contiguous and zero-padded like `001`.
2. Keep the migration **additive**. It must be correct when applied together
   with every earlier pending migration in one `migrate-up`, against
   existing data, while API servers and retention policy agents of earlier
   releases are still running and writing. Concretely:
   - Add columns to existing tables as nullable or with a default; never
     `SET NOT NULL` later.
   - Add nothing to `results` or `records` that an earlier release's writes
     could violate, and no new constraint of any kind on them: no unique
     index, `CHECK`, foreign key or trigger. New indexes on existing tables
     are non-unique and created `CONCURRENTLY`, one statement per file.
   - A new table that references `results` or `records` declares its foreign
     key `ON DELETE CASCADE` (or `ON DELETE SET NULL`): the retention policy
     agent of earlier releases deletes parent rows without knowing about the
     new table. `NOT VALID` does not help; it skips validating existing rows
     but still enforces the constraint on new writes and deletes.
   - Foreign keys between new tables that must hold for existing data are
     created `NOT VALID` and validated later by an administrative command,
     not by a migration.
   - Do not drop or rename objects that earlier releases use.
   - Make every statement idempotent: `CREATE TABLE IF NOT EXISTS`,
     `ADD COLUMN IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`,
     `CREATE OR REPLACE FUNCTION`.

   The restriction is narrow on purpose: `results` and `records` are the
   only tables that writers of the previous release touch. New tables may
   use `NOT NULL`, `CHECK`, unique and foreign-key constraints freely, and
   new columns may carry a `CHECK` that accepts `NULL`. Integrity that an
   older writer cannot satisfy is enforced by the new writers, established
   by backfill and proven by verification before dependent features
   activate; it is not expressed in the schema during the transition.

   A change that cannot meet these rules (`SET NOT NULL`, a unique
   constraint over existing rows, a column type change, dropping a column)
   is not a numbered migration. A numbered migration has one precondition,
   "the previous version is applied", and must succeed unattended from any
   source version while older writers run. Such a change has more (every
   writer on a release ≥ X, backfill complete, verification passed), so it
   is implemented as a separate `results-admin` operation that checks those
   preconditions and refuses otherwise, with `--writes-paused` as the
   operator's assertion where a precondition cannot be read from the
   database. No such operation exists yet; build one when a change needs it.
3. Raise `RequiredSchemaVersion` in `migrations.go` to `NNN` **only if**
   code in the same release depends on objects the migration creates. Point
   it at the last migration whose objects the release's writers need
   (tables, columns), never at an index migration: index builds can take
   long and must not block the release from starting. Index-dependent
   features gate on index validity at runtime instead.

   Raising the version is a user-visible event. A release that does so must
   say so in its release notes, because a failed migration then blocks the
   new release's pods from starting (see
   [Releases that raise the required version](../../../../../docs/database-migrations.md#releases-that-raise-the-required-version)).
4. Do not change the expectations in `structure.go`. They describe the
   schema that releases without versioned migrations created and are used to
   adopt such databases; rule 2 guarantees that no migration adds objects
   they would reject (they tolerate extra tables, nullable or defaulted
   columns and non-unique indexes).
5. Write `NNN_name.down.sql` as the exact inverse, also idempotent
   (`DROP ... IF EXISTS`). It is used by tests and by `migrate-to` in
   emergencies; releases never downgrade the schema.

Published migration files are never edited, renamed or deleted; fix a
mistake with a new migration. CI rejects pull requests that modify an
existing `.sql` file
([`presubmit-ci.yaml`](../../../../../.github/workflows/presubmit-ci.yaml),
"Check published migrations are unchanged").

## Tests

- `go test ./pkg/api/server/db/migrations/` runs without a database:
  `TestEmbeddedMigrations` (contiguous versions, up/down pairs,
  `RequiredSchemaVersion` within range), `TestMigrationsAreAdditive` (the
  rules of step 2 above, enforced by `lintMigration` on every up migration
  after the baseline) and unit tests for the compatibility rule and
  connection handling.
- `postgres_test.go` exercises adoption, drift detection, dirty state,
  `force`, lock timeouts and the real migrations against PostgreSQL:

  ```sh
  DB_URL="host=localhost port=5432 user=postgres password=postgres dbname=postgres sslmode=disable" \
    go test -tags=e2e ./pkg/api/server/db/migrations/
  ```

  Add a PostgreSQL test for every new migration that does more than add a
  table or column, in particular for `CONCURRENTLY` index builds and for
  behaviour of the previous release's code against the new schema.
- CI runs both, plus the end-to-end install which waits for the
  `tekton-results-db-migrate` Job.
