# Postgres Migrate

This script can be used to migrate from legacy MySQL database to the new
Postgres schema.

Note: The files in `config` are intended for local testing only - these set
plaintext passwords and store data locally to a Pod and not suitable for
production use.

## Usage

```sh
Usage of postgres-migrate:
  -write
        enable migration writes. if disabled, the tool still prints a summary of what would be migrated.
```

When ran, the migration tool will print out a summary of all objects in the old
MySQL database, along with the migration status of the script:

| Status         | Description                                                                                                                                                                                   |
| -------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| SUCCESS        | The entity was successfully written to the Postgres database                                                                                                                                  |
| ALREADY_EXISTS | The entity with the same name already exists in the new database. The script defers to this version as the latest source of truth.                                                            |
| WRITE_DISABLED | The entity would have been written to the Postgres database, but the `--write` flag was not provided. This is useful for checking what will be migrated prior to performing the actual write. |

Sample:

```json
{
  "a/results/fully-migrated": "ALREADY_EXISTS",
  "a/results/fully-migrated/records/pipelinerun-migrated": "ALREADY_EXISTS",
  "a/results/fully-migrated/records/taskrun-migrated": "ALREADY_EXISTS",
  "a/results/not-migrated": "ALREADY_EXISTS",
  "a/results/not-migrated/records/pipelinerun-not-migrated": "ALREADY_EXISTS",
  "a/results/not-migrated/records/taskrun-not-migrated": "ALREADY_EXISTS",
  "a/results/partially-migrated": "ALREADY_EXISTS",
  "a/results/partially-migrated/records/pipelinerun-migrated": "ALREADY_EXISTS",
  "a/results/partially-migrated/records/taskrun-not-migrated": "ALREADY_EXISTS"
}
```

If errors occur during migration, the tool may still perform a partial
migration. Check the output summary to see if entities were migrated.

The tool is intended to be safely ran multiple times - if the entity was already
migrated no further action will be taken.

The tool does **not** re-enqueue any work to the watcher reconciler - the
expectation is that the new postgres watcher is already up and running to handle
in-flight / new Runs.
