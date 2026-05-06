# Migration Kit

The `kit` package contains Grizzle's schema migration and code-generation tooling.

Grizzle is moving toward a Drizzle Kit RC.1-style file-migration workflow:

```text
schema definitions
  -> grizzle generate (runs pre-check)
  -> review generated artifacts
  -> grizzle check
  -> grizzle migrate
```

In that target workflow, migration artifacts are generated as files, committed to source control, validated before use, and then applied by `migrate`.

## Target File-Migration Workflow

The target public commands are:

| Command | Role |
|---|---|
| `grizzle generate` | Convert Go schema definitions into migration artifacts under `./grizzle` |
| `grizzle check` | Validate migration artifacts, snapshots, ordering, and branch consistency |
| `grizzle migrate` | Apply pending committed migration artifacts to a database |
| `grizzle push` | Public direct-sync command boundary; full safety contract belongs to a dedicated push spec |
| `grizzle pull` | Introspect a live database into Go schema definition source |
| `grizzle introspect` | Alias of `grizzle pull` |

The target file-migration history table defaults to `__grizzle_migrations`. PostgreSQL uses the default migration schema `grizzle`. Those Grizzle-branded default names are **DEVIATION:INTENTIONAL** namespace/branding divergence from RC.1's `__drizzle_migrations` / `drizzle` defaults; row schema and migration behavior still target RC.1.

The detailed target contract lives in [the migration specs](../spec/kit.md). Those specs are pinned to Drizzle ORM / Drizzle Kit `v1.0.0-rc.1`.

## Current Live-Diff Helpers

Some existing `kit` APIs are current implementation surface, not the final file-migration deployment model.

| API | Current role |
|---|---|
| `kit.Push` | Diff live database state against schema definitions and apply DDL directly |
| `kit.DryRun` | Diff live database state against schema definitions and return SQL without applying it |
| `kit.Migrate` | Current flat `.sql` file runner with legacy history recording; broken relative to the RC.1 folder-per-migration target |
| `kit.Status` | Legacy/current history and pending-change reporting helper |

The current live-diff history table name is legacy implementation detail. New RC.1-aligned file-migration work must use the `__grizzle_migrations` contract specified in [file-migrations-history.md](../spec/file-migrations-history.md).

Current direct-sync helpers, including `kit.Push`, are development/control-plane tools in the current implementation. They are not deployment-safe defaults for shared environments and must not be expanded or recommended for production-style use until a dedicated push/direct-sync spec defines locking, destructive-change handling, dry-run behavior, and non-interactive safety.

## Push

`push` remains a direct-apply shortcut in the public command model, but it is not specified by the file-migration documents because it does not create, validate, or apply migration artifacts.

Before new RC.1-aligned `push` work resumes, a dedicated direct-sync spec must define destructive-change handling, force behavior, dry-run behavior, locking, non-interactive behavior, and CI safety. Production-style deployment should use generated, reviewed, committed artifacts with `check` and `migrate`.

## Generate

`grizzle generate` is the target schema-to-migration-artifact command. It must:

- run `check` before writing artifacts
- compare current schema definitions against the selected prior snapshot context
- resolve ambiguous renames through Drizzle RC.1-style interactive prompts
- write a migration directory containing `migration.sql` and `snapshot.json`

## Check

`grizzle check` validates the local migration artifact set. It is an offline command for the initial design and does not connect to the database, even if shared config contains database credentials.

## Migrate

`grizzle migrate` is the target artifact execution command. It must apply pending migration directories from the configured migrations output directory and record successful applications in `__grizzle_migrations`.

The target runner uses migration directory names as identity and explicit `--> statement-breakpoint` markers for statement segmentation. It does not scan arbitrary root-level `.sql` files.

## Pull

`grizzle pull` connects to a live database, introspects schema state, and writes Go schema definition files such as `schema.go` and `relations.go`.

`pull` is the Go equivalent of Drizzle Kit `pull` / `introspect`. It is not the same command as `grizzle gen`.

Filesystem-mutating `pull` requires schema/table filters or an explicit `--all-schemas` / `AllowBroadScan` opt-in before broad introspection. Prefer filters for shared databases, and review generated files because object names and SQL literals come from the live database.

## Gen

`grizzle gen` reads existing Go schema definition files and emits typed Go helper code such as table structs, select structs, insert structs, update structs, and typed column handles.

```sh
grizzle gen --schema ./schema --out ./schema --package schema
```

`gen` exists because Go needs generated helper types where TypeScript can infer them directly. It does not connect to a live database.

See [codegen.md](../spec/codegen.md) for the authoritative `grizzle gen` contract.
